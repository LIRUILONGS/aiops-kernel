package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"opsagi/pkg/config"
)

// PromQueryArgs is a range query against a Prometheus-compatible API. The window
// is relative to now, because an agent investigating an incident does not know
// absolute timestamps and guessing them wastes a round trip.
type PromQueryArgs struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"PromQL 表达式，例如 rate(container_cpu_usage_seconds_total{name=\"caddy\"}[5m]) 或 node_load1"`
	Range string `json:"range,omitempty" jsonschema:"description=往回看多久，Go 时长格式，默认 15m，范围 1m~24h,example=30m"`
	Step  string `json:"step,omitempty" jsonschema:"description=采样步长，默认按窗口均分 30 段，范围 1s~1h,example=30s"`
}

const (
	promDefaultRange = 15 * time.Minute
	promMinRange     = time.Minute
	promMaxRange     = 24 * time.Hour
	promMaxPoints    = 11000
	promMaxQueryLen  = 2000
)

type promTools struct {
	baseURL string
	client  *http.Client
}

// NewPromTools builds the metrics tool from configuration. No URL means the tool
// stays unavailable and is dropped from every agent's tool list, so a model can
// never see a metrics tool that has nowhere to query.
func NewPromTools(cfg *config.Config) *promTools {
	tools := &promTools{client: &http.Client{Timeout: 30 * time.Second}}
	if cfg == nil {
		return tools
	}

	tools.baseURL = strings.TrimRight(strings.TrimSpace(cfg.PrometheusURL), "/")

	return tools
}

func (p *promTools) IsAvailable() bool { return p.baseURL != "" }

func (p *promTools) HandleQuery(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	var parsed PromQueryArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}

	query, window, step, err := p.normalize(parsed)
	if err != nil {
		return "", err
	}

	end := time.Now()
	values := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(end.Add(-window).Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.FormatInt(int64(step.Seconds()), 10)},
	}

	requestURL := p.baseURL + "/api/v1/query_range?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造查询请求失败：%w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询 %s 失败：%w", p.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("读取查询响应失败：%w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询被拒绝（HTTP %d）：%s", resp.StatusCode, promErrorText(body))
	}

	var payload promResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析查询响应失败：%w", err)
	}
	if payload.Status != "success" {
		return "", fmt.Errorf("查询失败：%s", promErrorText(body))
	}

	return renderPromResult(query, window, step, end, payload.Data), nil
}

// normalize rejects a window or step the server would refuse anyway, with a
// message that says what to send instead of the HTTP status it would have got.
func (p *promTools) normalize(args PromQueryArgs) (string, time.Duration, time.Duration, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", 0, 0, fmt.Errorf("query 为空：写一条 PromQL 表达式")
	}
	if len(query) > promMaxQueryLen {
		return "", 0, 0, fmt.Errorf("query 长度 %d 超过上限 %d：拆成几条查询分别执行", len(query), promMaxQueryLen)
	}

	window := promDefaultRange
	if strings.TrimSpace(args.Range) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(args.Range))
		if err != nil {
			return "", 0, 0, fmt.Errorf("range=%q 不是合法时长：用 15m、1h 这样的写法", args.Range)
		}
		window = parsed
	}
	if window < promMinRange || window > promMaxRange {
		return "", 0, 0, fmt.Errorf("range=%s 超出 1m~24h：拐点通常出现在最近一小时里，先缩小窗口再放大", window)
	}

	step := window / 30
	if strings.TrimSpace(args.Step) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(args.Step))
		if err != nil {
			return "", 0, 0, fmt.Errorf("step=%q 不是合法时长：用 30s、1m 这样的写法", args.Step)
		}
		step = parsed
	}
	if step < time.Second {
		step = time.Second
	}
	if points := int(window / step); points > promMaxPoints {
		return "", 0, 0, fmt.Errorf("窗口 %s 按步长 %s 会取 %d 个点，超过上限 %d：加大步长或缩小窗口", window, step, points, promMaxPoints)
	}

	return query, window, step, nil
}

type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
	Error  string   `json:"error"`
}

type promData struct {
	ResultType string      `json:"resultType"`
	Result     []promSerie `json:"result"`
}

type promSerie struct {
	Metric map[string]string `json:"metric"`
	Values [][2]any          `json:"values"`
	Value  []any             `json:"value"`
}

func promErrorText(body []byte) string {
	var payload promResponse
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}

	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	if text == "" {
		return "响应为空"
	}

	return text
}

// renderPromResult turns a matrix into what an operator needs to judge a change:
// the endpoints of each series, its extremes, and where it moved most. The jump
// is computed here because "拐点" is arithmetic, and a model doing it in prose
// gets it wrong as soon as there are two series.
func renderPromResult(query string, window, step time.Duration, end time.Time, data promData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "监控查询：%s\n", query)
	fmt.Fprintf(&b, "窗口：%s 到 %s（步长 %s，%s）\n",
		end.Add(-window).Format("2006-01-02T15:04:05Z07:00"),
		end.Format("2006-01-02T15:04:05Z07:00"),
		step,
		durationText(window),
	)

	if len(data.Result) == 0 {
		b.WriteString("\n窗口内没有数据。可能是选择器写错、目标没被采集，或这段时间确实没有样本；先换一个更宽的窗口或去掉标签再试一次。\n")

		return b.String()
	}

	fmt.Fprintf(&b, "序列数：%d\n", len(data.Result))

	for i, serie := range data.Result {
		points := serie.points()
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, promSeriesName(serie.Metric))
		if len(points) == 0 {
			b.WriteString("   窗口内没有样本\n")
			continue
		}

		first, last := points[0], points[len(points)-1]
		minPoint, maxPoint := points[0], points[0]

		for _, point := range points {
			if point.value < minPoint.value {
				minPoint = point
			}
			if point.value > maxPoint.value {
				maxPoint = point
			}
		}

		fmt.Fprintf(&b, "   点数 %d，首值 %.4g，末值 %.4g（%s），最小 %.4g @%s，最大 %.4g @%s\n",
			len(points), first.value, last.value, changeText(first.value, last.value),
			minPoint.value, minPoint.at.Format("15:04:05"), maxPoint.value, maxPoint.at.Format("15:04:05"))

		if jump, ok := largestJump(points); ok {
			fmt.Fprintf(&b, "   最大跳变 %s：%s %.4g → %.4g @%s\n",
				changeText(jump.fromValue, jump.toValue),
				jump.from.Format("15:04:05"), jump.fromValue, jump.toValue, jump.to.Format("15:04:05"))
		}
	}

	return b.String()
}

type promPoint struct {
	at    time.Time
	value float64
}

func (s promSerie) points() []promPoint {
	raw := s.Values
	if len(raw) == 0 && len(s.Value) == 2 {
		raw = [][2]any{{s.Value[0], s.Value[1]}}
	}

	points := make([]promPoint, 0, len(raw))
	for _, pair := range raw {
		at, value, ok := promPair(pair)
		if !ok {
			continue
		}
		points = append(points, promPoint{at: at, value: value})
	}

	sort.Slice(points, func(i, j int) bool { return points[i].at.Before(points[j].at) })

	return points
}

func promPair(pair [2]any) (time.Time, float64, bool) {
	rawTS, ok := pair[0].(float64)
	if !ok {
		return time.Time{}, 0, false
	}

	rawValue, ok := pair[1].(string)
	if !ok {
		return time.Time{}, 0, false
	}

	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return time.Time{}, 0, false
	}

	return time.Unix(int64(rawTS), 0), value, true
}

type promJump struct {
	from, to           time.Time
	fromValue, toValue float64
}

// largestJump finds the adjacent pair with the largest relative change, which is
// where a change landed rather than where the value happens to be highest.
func largestJump(points []promPoint) (promJump, bool) {
	if len(points) < 2 {
		return promJump{}, false
	}

	var best promJump
	var bestRatio float64

	for i := 1; i < len(points); i++ {
		previous, current := points[i-1], points[i]
		denominator := math.Abs(previous.value)
		if denominator == 0 {
			denominator = 1
		}

		ratio := math.Abs(current.value-previous.value) / denominator
		if ratio > bestRatio {
			bestRatio = ratio
			best = promJump{
				from:      previous.at,
				to:        current.at,
				fromValue: previous.value,
				toValue:   current.value,
			}
		}
	}

	return best, bestRatio > 0
}

func promSeriesName(metric map[string]string) string {
	name := metric["__name__"]

	labels := make([]string, 0, len(metric))
	for key, value := range metric {
		if key == "__name__" {
			continue
		}
		labels = append(labels, fmt.Sprintf("%s=%q", key, value))
	}
	sort.Strings(labels)

	if len(labels) == 0 {
		if name == "" {
			return "(无标签)"
		}

		return name
	}

	if name == "" {
		return "{" + strings.Join(labels, ", ") + "}"
	}

	return name + "{" + strings.Join(labels, ", ") + "}"
}

func durationText(window time.Duration) string {
	if window%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d 天", int(window/(24*time.Hour)))
	}

	return window.String()
}

func changeText(from, to float64) string {
	switch {
	case from == 0 && to == 0:
		return "持平"
	case from == 0:
		return fmt.Sprintf("从 0 起，%.4g", to)
	}

	change := (to - from) / math.Abs(from) * 100
	trend := "持平"
	if change > 0.5 {
		trend = "上升"
	} else if change < -0.5 {
		trend = "下降"
	}

	return fmt.Sprintf("%+.1f%% %s", change, trend)
}
