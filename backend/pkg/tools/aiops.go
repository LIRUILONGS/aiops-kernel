package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The exit schemas of the AIOps roles.
//
// A field's shape is a requirement, not a decoration: an evidence array makes
// the model enumerate, a pointer makes an omitted field distinguishable from a
// false one, and Validate refuses an incomplete object so the tool-call fixer
// asks again instead of the chain concluding on a guess.
//
// Every message below is read by the model, so it says what to write next
// rather than only what is wrong.

// evidenceSources lists the data sources a diagnosis may cite. A source the
// runtime has no tool for is not a source: the model would cite it and no one
// could reproduce the excerpt.
var evidenceSources = []string{"host", "containers", "logs", "metrics", "changes", "k8s"}

type EvidenceItem struct {
	Source    string `json:"source" jsonschema:"required,type=string" jsonschema_description:"证据来自哪一路：host=主机快照，containers=容器状态与重启次数，logs=容器或系统日志，metrics=指标，changes=发布与配置变更，k8s=集群对象"`
	Query     string `json:"query" jsonschema:"required" jsonschema_description:"你实际调用的工具及其参数（例如 ops_logs source=opslab-broken lines=200），别人靠它复现这条证据"`
	Timestamp string `json:"timestamp" jsonschema:"required" jsonschema_description:"证据对应的时刻（RFC3339，或与告警同一时区的 HH:MM:SS），必须与告警时间对齐"`
	Excerpt   string `json:"excerpt" jsonschema:"required" jsonschema_description:"工具返回的原文片段，可截断到 500 字以内；不要转述、不要改写、不要补全"`
}

type DiagnosisRecommendation struct {
	Action   string `json:"action" jsonschema:"required" jsonschema_description:"人下一步应该做什么，具体到命令或操作对象；确认不需要动作也要写「无需动作」"`
	Risk     string `json:"risk" jsonschema:"required,type=string" jsonschema_description:"建议动作的风险等级：low / medium / high / unknown"`
	Rollback string `json:"rollback" jsonschema:"required" jsonschema_description:"怎么撤销这次建议动作；无需撤销也要写「无需撤销」"`
}

type Diagnosis struct {
	RootCause      string                  `json:"root_cause" jsonschema:"required" jsonschema_description:"一句话根因：哪个组件、什么机制失效；不要复述现象，也不要写「可能是」"`
	Evidence       []EvidenceItem          `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少两条能靠 query 复现的证据；结论里的每一句判断都要能追到其中一条"`
	Confidence     *float64                `json:"confidence" jsonschema:"type=number" jsonschema_description:"0 到 1 之间的置信度，诚实填写；0.4 分但写清还缺什么证据，比 0.9 分却是错的更有用"`
	BlastRadius    string                  `json:"blast_radius" jsonschema:"required" jsonschema_description:"影响谁、多大范围、从哪一刻开始"`
	Recommendation DiagnosisRecommendation `json:"recommendation" jsonschema:"required" jsonschema_description:"给人看的处置建议，不是你要执行的动作"`
	NeedsHuman     *Bool                   `json:"needs_human" jsonschema:"type=boolean" jsonschema_description:"除非结论完整且建议动作是「无需动作」，否则填 true"`
	Message        string                  `json:"message,omitempty" jsonschema_description:"一句话交代这次诊断的结论走向，写入事故记录"`
}

// OptimizationResult is the exit of a performance task. It exists because an
// optimisation that is not measured is a guess: baseline and observed_gain are
// required fields, and "no measurable effect" is a legal verdict rather than a
// failure the model has to talk its way out of.
type OptimizationResult struct {
	Bottleneck      string         `json:"bottleneck" jsonschema:"required" jsonschema_description:"瓶颈是什么，靠哪条证据定位：CPU 饱和、内存压力、磁盘 IO、网络、锁竞争、GC、慢查询、串行化，还是配置不当"`
	Baseline        string         `json:"baseline" jsonschema:"required" jsonschema_description:"变更前的测量结果：ops_baseline 或监控给出的关键数值与采集时刻；没有基线就不要提出改动"`
	Proposal        string         `json:"proposal" jsonschema:"required" jsonschema_description:"具体改动：改哪个文件、哪个参数、改成什么值。写清楚到别人能照着执行"`
	ExpectedGain    string         `json:"expected_gain" jsonschema:"required" jsonschema_description:"预期改善哪个指标、改善多少、依据是什么"`
	ObservedGain    string         `json:"observed_gain,omitempty" jsonschema_description:"变更后用 ops_diff 复测到的实际变化。verdict 不是 unverified 时必填（那三个判定都表示已经复测），verdict=unverified 时必须留空"`
	MeasuredAgainst string         `json:"measured_against,omitempty" jsonschema_description:"这次复测对照的目标：把用户给的指标与目标值原样抄进来（例如「p99 延迟 < 200ms」）。flow 声明了【性能目标】且 verdict 不是 unverified 时必填"`
	Verdict         string         `json:"verdict" jsonschema:"required,type=string" jsonschema_description:"结论：improved=有可测量改善，no_measurable_effect=测过但没变化（这是合法结论），degraded=变差了，unverified=没有复测条件"`
	Rollback        string         `json:"rollback" jsonschema:"required" jsonschema_description:"怎么把这次改动改回去；无需回滚也要写「无需回滚」"`
	Evidence        []EvidenceItem `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少两条能靠 query 复现的证据：基线一条、结论一条"`
	Confidence      *float64       `json:"confidence" jsonschema:"type=number" jsonschema_description:"0 到 1 之间的置信度，按基线与复测的可信度给，不要按改动的自信程度给"`
	Message         string         `json:"message,omitempty" jsonschema_description:"一句话交代这次优化的结论走向，写入性能记录"`
}

type TriageResult struct {
	Severity     string         `json:"severity" jsonschema:"required,type=string" jsonschema_description:"分级：critical / high / medium / low / info"`
	BlastRadius  string         `json:"blast_radius" jsonschema:"required" jsonschema_description:"影响谁、多大范围、从哪一刻开始"`
	KnownPattern *Bool          `json:"known_pattern" jsonschema:"type=boolean" jsonschema_description:"是否命中已知故障模式；命中时在 result 里写出模式名或历史事故号"`
	Result       string         `json:"result" jsonschema:"required" jsonschema_description:"完整技术报告：判断、依据、还缺什么。这段文字是事故指挥唯一能看到的内容"`
	Evidence     []EvidenceItem `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少一条能靠 query 复现的证据"`
	Message      string         `json:"message,omitempty" jsonschema_description:"一句话交给事故指挥"`
}

type LogFinding struct {
	Pattern  string         `json:"pattern" jsonschema:"required" jsonschema_description:"错误模式：正则可直接匹配的原文特征，而不是「报错很多」这类概括"`
	Onset    string         `json:"onset" jsonschema:"required" jsonschema_description:"这个错误模式首次出现的时刻，必须与告警时间对齐"`
	Result   string         `json:"result" jsonschema:"required" jsonschema_description:"完整技术报告：查了哪些日志、模式是什么、首次出现时间、与告警的先后关系"`
	Evidence []EvidenceItem `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少一条能靠 query 复现的证据"`
	Message  string         `json:"message,omitempty" jsonschema_description:"一句话交给事故指挥"`
}

type MetricFinding struct {
	Metric   string         `json:"metric" jsonschema:"required" jsonschema_description:"指标名与标签，例如 container_memory_working_set_bytes{pod=\"payments-api\"}"`
	Onset    string         `json:"onset" jsonschema:"required" jsonschema_description:"拐点时刻（数值开始异常的那一点），不是发现异常的时刻"`
	Result   string         `json:"result" jsonschema:"required" jsonschema_description:"完整技术报告：查了哪些指标、拐点前后数值、与告警的先后关系"`
	Evidence []EvidenceItem `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少一条能靠 query 复现的证据"`
	Message  string         `json:"message,omitempty" jsonschema_description:"一句话交给事故指挥"`
}

type ChangeFinding struct {
	Change   string         `json:"change" jsonschema:"required" jsonschema_description:"谁在什么时候改了什么：发布版本、配置项、依赖升级或扩缩容"`
	Onset    string         `json:"onset" jsonschema:"required" jsonschema_description:"变更生效时刻；与告警时间的间隔要写进 result"`
	Result   string         `json:"result" jsonschema:"required" jsonschema_description:"完整技术报告：查了哪些变更记录、时间线、与告警的先后关系"`
	Evidence []EvidenceItem `json:"evidence" jsonschema:"required,type=array" jsonschema_description:"至少一条能靠 query 复现的证据"`
	Message  string         `json:"message,omitempty" jsonschema_description:"一句话交给事故指挥"`
}

func (d Diagnosis) Validate() error {
	if strings.TrimSpace(d.RootCause) == "" {
		return fmt.Errorf("root_cause 为空：写清是哪个组件、什么机制失效，而不是复述现象")
	}
	if err := validateEvidence(d.Evidence, 2); err != nil {
		return err
	}
	if d.Confidence == nil {
		return fmt.Errorf("confidence 缺失：填 0 到 1 之间的数值；不确定就写低分并说明还缺什么证据")
	}
	if c := *d.Confidence; c < 0 || c > 1 {
		return fmt.Errorf("confidence=%v 超出 0 到 1 的范围", c)
	}
	if strings.TrimSpace(d.BlastRadius) == "" {
		return fmt.Errorf("blast_radius 为空：写清影响谁、多大范围、从哪一刻开始")
	}
	if strings.TrimSpace(d.Recommendation.Action) == "" {
		return fmt.Errorf("recommendation.action 为空：写人下一步该做什么；确认不用动也要写「无需动作」")
	}
	if err := validateRisk(d.Recommendation.Risk); err != nil {
		return err
	}
	if strings.TrimSpace(d.Recommendation.Rollback) == "" {
		return fmt.Errorf("recommendation.rollback 为空：写清怎么撤销这个建议动作；无需撤销也要写「无需撤销」")
	}
	if d.NeedsHuman == nil {
		return fmt.Errorf("needs_human 缺失：除非结论完整且建议动作是「无需动作」，否则填 true")
	}

	return nil
}

func (r TriageResult) Validate() error {
	if err := validateEnum("severity", r.Severity, []string{"critical", "high", "medium", "low", "info"}); err != nil {
		return err
	}
	if strings.TrimSpace(r.BlastRadius) == "" {
		return fmt.Errorf("blast_radius 为空：写清影响谁、多大范围、从哪一刻开始")
	}
	if r.KnownPattern == nil {
		return fmt.Errorf("known_pattern 缺失：命中已知故障模式填 true 并写出模式名，否则填 false")
	}
	if err := validateFindingBody("triage", r.Result, r.Evidence); err != nil {
		return err
	}

	return nil
}

func (f LogFinding) Validate() error {
	if strings.TrimSpace(f.Pattern) == "" {
		return fmt.Errorf("pattern 为空：写工具有返回、能被正则直接匹配的原文特征")
	}
	if strings.TrimSpace(f.Onset) == "" {
		return fmt.Errorf("onset 为空：写这个错误模式首次出现的时刻")
	}
	return validateFindingBody("log", f.Result, f.Evidence)
}

func (f MetricFinding) Validate() error {
	if strings.TrimSpace(f.Metric) == "" {
		return fmt.Errorf("metric 为空：写指标名与标签，别人靠它复现查询")
	}
	if strings.TrimSpace(f.Onset) == "" {
		return fmt.Errorf("onset 为空：写数值开始异常的那一刻，而不是你发现它的时刻")
	}
	return validateFindingBody("metric", f.Result, f.Evidence)
}

func (f ChangeFinding) Validate() error {
	if strings.TrimSpace(f.Change) == "" {
		return fmt.Errorf("change 为空：写谁在什么时候改了什么")
	}
	if strings.TrimSpace(f.Onset) == "" {
		return fmt.Errorf("onset 为空：写变更生效的时刻")
	}
	return validateFindingBody("change", f.Result, f.Evidence)
}

func validateFindingBody(role, result string, evidence []EvidenceItem) error {
	if strings.TrimSpace(result) == "" {
		return fmt.Errorf("%s 的 result 为空：写出完整技术报告（查了什么、看到什么、结论是什么），这是事故指挥唯一能看到的输入", role)
	}
	return validateEvidence(evidence, 1)
}

func validateEvidence(evidence []EvidenceItem, want int) error {
	if len(evidence) < want {
		return fmt.Errorf("evidence 只有 %d 条，至少 %d 条：每条要能在同名 source 上用 query 复现", len(evidence), want)
	}
	for i, item := range evidence {
		if err := validateEnum(fmt.Sprintf("evidence[%d].source", i), item.Source, evidenceSources); err != nil {
			return err
		}
		if strings.TrimSpace(item.Query) == "" {
			return fmt.Errorf("evidence[%d].query 为空：写你实际调用的工具和参数", i)
		}
		if strings.TrimSpace(item.Timestamp) == "" {
			return fmt.Errorf("evidence[%d].timestamp 为空：写证据对应的时刻，与告警时间对齐", i)
		}
		if strings.TrimSpace(item.Excerpt) == "" {
			return fmt.Errorf("evidence[%d].excerpt 为空：贴工具返回的原文片段，不要转述", i)
		}
	}

	return nil
}

func validateRisk(risk string) error {
	return validateEnum("recommendation.risk", risk, []string{"low", "medium", "high", "unknown"})
}

// validateEnum reports an out-of-set value together with the set itself, so the
// model can pick one instead of retrying the same word.
func validateEnum(field, value string, allowed []string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}

	return fmt.Errorf("%s=%q 不在允许的值里：%s", field, value, strings.Join(allowed, " / "))
}

// Render is the text a diagnosis becomes once accepted: the subtask result, the
// report message and therefore the only place the evidence survives. Markdown
// first because a human reads it; the JSON block at the end keeps the fields
// verbatim for whatever parses the row later.
func (d Diagnosis) Render() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}

	var b strings.Builder

	b.WriteString("## 诊断结论\n\n")
	fmt.Fprintf(&b, "**根因**：%s\n\n", d.RootCause)
	fmt.Fprintf(&b, "**置信度**：%.2f\n\n", *d.Confidence)
	fmt.Fprintf(&b, "**影响面**：%s\n\n", d.BlastRadius)
	if d.NeedsHuman.Bool() {
		b.WriteString("**需要人工介入**：是\n\n")
	} else {
		b.WriteString("**需要人工介入**：否\n\n")
	}

	b.WriteString("### 证据\n\n")
	for i, item := range d.Evidence {
		fmt.Fprintf(&b, "%d. **%s** · %s · `%s`\n\n", i+1, item.Source, item.Timestamp, item.Query)
		for _, line := range strings.Split(item.Excerpt, "\n") {
			fmt.Fprintf(&b, "   > %s\n", strings.TrimRight(line, "\r"))
		}
		b.WriteString("\n")
	}

	b.WriteString("### 建议动作\n\n")
	fmt.Fprintf(&b, "- **动作**：%s\n", d.Recommendation.Action)
	fmt.Fprintf(&b, "- **风险**：%s\n", d.Recommendation.Risk)
	fmt.Fprintf(&b, "- **回滚**：%s\n\n", d.Recommendation.Rollback)

	payload, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("序列化诊断失败：%w", err)
	}
	b.WriteString("### 结构化结果\n\n```json\n")
	b.Write(payload)
	b.WriteString("\n```\n")

	return b.String(), nil
}

func (o OptimizationResult) Validate() error {
	if strings.TrimSpace(o.Bottleneck) == "" {
		return fmt.Errorf("bottleneck 为空：写清瓶颈是哪个资源或哪段代码，而不是「系统有点慢」")
	}
	if strings.TrimSpace(o.Baseline) == "" {
		return fmt.Errorf("baseline 为空：没有变更前的测量结果，这次优化无法被验证")
	}
	if strings.TrimSpace(o.Proposal) == "" {
		return fmt.Errorf("proposal 为空：写清改哪个参数、改成什么值")
	}
	if strings.TrimSpace(o.ExpectedGain) == "" {
		return fmt.Errorf("expected_gain 为空：写清预期改善哪个指标、改善多少")
	}
	if err := validateEnum("verdict", o.Verdict, []string{"improved", "no_measurable_effect", "degraded", "unverified"}); err != nil {
		return err
	}
	// The verdict claims whether a measurement happened, and observed_gain is that
	// measurement. The two can disagree in both directions, and either way the
	// report would read as a result it does not have.
	if err := validateObservedGain(o.Verdict, o.ObservedGain); err != nil {
		return err
	}
	if strings.TrimSpace(o.Rollback) == "" {
		return fmt.Errorf("rollback 为空：写清怎么改回去；无需回滚也要写「无需回滚」")
	}
	if err := validateEvidence(o.Evidence, 2); err != nil {
		return err
	}
	if o.Confidence == nil {
		return fmt.Errorf("confidence 缺失：填 0 到 1 之间的数值")
	}
	if c := *o.Confidence; c < 0 || c > 1 {
		return fmt.Errorf("confidence=%v 超出 0 到 1 的范围", c)
	}
	return nil
}

// PerformanceGoal is the target a user declared when the flow was created, as it
// appears in the 【性能目标】 block the web form appends to the input.
type PerformanceGoal struct {
	Constraint string
	Metric     string
	Target     string
}

// Empty reports whether the flow declared no goal at all, in which case every
// goal check is skipped rather than guessed.
func (g PerformanceGoal) Empty() bool {
	return g.Metric == "" && g.Target == "" && g.Constraint == ""
}

func (g PerformanceGoal) String() string {
	var parts []string

	if g.Metric != "" {
		parts = append(parts, "指标 "+g.Metric)
	}
	if g.Target != "" {
		parts = append(parts, "目标 "+g.Target)
	}
	if g.Constraint != "" {
		parts = append(parts, "约束 "+g.Constraint)
	}

	return strings.Join(parts, "，")
}

const performanceGoalMarker = "【性能目标】"

// ParsePerformanceGoal reads the goal block out of a flow input. It accepts the
// block wherever it sits in the message, because a user may type it by hand as
// well as get it from the form.
func ParsePerformanceGoal(input string) PerformanceGoal {
	var goal PerformanceGoal

	lines := strings.Split(input, "\n")
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.Contains(trimmed, performanceGoalMarker):
			inBlock = true
			continue
		case inBlock && strings.HasPrefix(trimmed, "【"):
			inBlock = false
		case !inBlock:
			continue
		}

		key, value := splitGoalLine(trimmed)
		switch key {
		case "指标":
			goal.Metric = value
		case "目标":
			goal.Target = value
		case "约束":
			goal.Constraint = value
		}
	}

	return goal
}

func splitGoalLine(line string) (string, string) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(line, "-"), "*")
	trimmed = strings.TrimSpace(trimmed)

	for _, sep := range []string{"：", ":"} {
		if key, value, found := strings.Cut(trimmed, sep); found {
			return strings.TrimSpace(key), strings.TrimSpace(value)
		}
	}

	return "", ""
}

// ValidateAgainst applies the goal the user declared to a conclusion. A goal is a
// constraint, so it belongs in the exit schema rather than in the prompt: a
// report cannot claim an improvement on a metric nobody asked about, and cannot
// claim one at all without a before/after comparison behind it.
func (o OptimizationResult) ValidateAgainst(goal PerformanceGoal) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if goal.Empty() || o.Verdict == "unverified" {
		return nil
	}
	if strings.TrimSpace(o.MeasuredAgainst) == "" {
		return fmt.Errorf("这个 flow 声明了性能目标（%s），所以 verdict=%s 时必须填 measured_against：把目标原样抄进来，再让 observed_gain 说明是否达标；确实没有复测就把 verdict 改成 unverified", goal.String(), o.Verdict)
	}
	if goal.Metric != "" && !strings.Contains(normalizeText(o.MeasuredAgainst), normalizeText(goal.Metric)) {
		return fmt.Errorf("measured_against=%q 没有提到这次 flow 要看的指标 %q：复测只能对着用户要的指标，换了指标就另开一个结论", o.MeasuredAgainst, goal.Metric)
	}
	if !o.citesDiff() {
		return fmt.Errorf("声明了性能目标就必须有一次 before/after 对比，而 evidence 里没有任何一条来自 ops_diff：先采 before，做完改动再采 after，用 ops_diff 算出差值，并把那次调用写进 query")
	}

	return nil
}

func (o OptimizationResult) citesDiff() bool {
	for _, item := range o.Evidence {
		if strings.Contains(strings.ToLower(item.Query), "ops_diff") {
			return true
		}
	}

	return false
}

func normalizeText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

// validateObservedGain keeps a claim of measurement and the measurement itself in
// agreement. Every verdict except unverified asserts that the change was measured,
// so each needs the numbers; unverified asserts the opposite, so it must not carry
// any.
func validateObservedGain(verdict, observed string) error {
	measured := strings.TrimSpace(observed) != ""

	if verdict == "unverified" {
		if measured {
			return fmt.Errorf("verdict=unverified 但 observed_gain 有内容：这个判定表示没有复测条件，就把 observed_gain 留空；如果你确实复测过，把 verdict 改成 improved / degraded / no_measurable_effect 并保留这段实测值")
		}

		return nil
	}

	if !measured {
		if verdict == "improved" {
			return fmt.Errorf("verdict=improved 但 observed_gain 为空：没有复测就不能算改善，改判 unverified 或补上复测结果")
		}

		return fmt.Errorf("verdict=%s 但 observed_gain 为空：这个判定表示已经复测过，把 ops_diff 的实际变化写进 observed_gain；没有复测条件就把 verdict 改成 unverified", verdict)
	}

	return nil
}

// Render is the text an optimisation becomes once accepted. The verdict is the
// headline: an operator needs to know whether the change worked before reading
// why.
func (o OptimizationResult) Render() (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}

	var b strings.Builder

	b.WriteString("## 性能优化结论\n\n")
	fmt.Fprintf(&b, "**结论**：%s\n\n", o.Verdict)
	fmt.Fprintf(&b, "**瓶颈**：%s\n\n", o.Bottleneck)
	fmt.Fprintf(&b, "**置信度**：%.2f\n\n", *o.Confidence)
	fmt.Fprintf(&b, "**变更前**：%s\n\n", o.Baseline)
	fmt.Fprintf(&b, "**建议改动**：%s\n\n", o.Proposal)
	fmt.Fprintf(&b, "**预期收益**：%s\n\n", o.ExpectedGain)
	if o.ObservedGain != "" {
		fmt.Fprintf(&b, "**实测收益**：%s\n\n", o.ObservedGain)
	} else {
		b.WriteString("**实测收益**：未复测\n\n")
	}
	fmt.Fprintf(&b, "**回滚**：%s\n\n", o.Rollback)
	if o.MeasuredAgainst != "" {
		fmt.Fprintf(&b, "**对照目标**：%s\n\n", o.MeasuredAgainst)
	}

	b.WriteString("### 证据\n\n")
	for i, item := range o.Evidence {
		fmt.Fprintf(&b, "%d. **%s** · %s · `%s`\n\n", i+1, item.Source, item.Timestamp, item.Query)
		for _, line := range strings.Split(item.Excerpt, "\n") {
			fmt.Fprintf(&b, "   > %s\n", strings.TrimRight(line, "\r"))
		}
		b.WriteString("\n")
	}

	payload, err := json.Marshal(o)
	if err != nil {
		return "", fmt.Errorf("序列化优化结论失败：%w", err)
	}
	b.WriteString("### 结构化结果\n\n```json\n")
	b.Write(payload)
	b.WriteString("\n```\n")

	return b.String(), nil
}

// renderFinding is the shape every specialist delegation returns: a headline
// field, the report, and the evidence the commander can quote into the final
// diagnosis without re-running anything.
func renderFinding(title, headline, label, onset, result, message string, evidence []EvidenceItem) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s\n\n", title)
	fmt.Fprintf(&b, "**%s**：%s\n\n", label, headline)
	if onset != "" {
		fmt.Fprintf(&b, "**时间点**：%s\n\n", onset)
	}
	b.WriteString(result)
	b.WriteString("\n")

	if len(evidence) > 0 {
		b.WriteString("\n### 证据\n\n")
		for i, item := range evidence {
			fmt.Fprintf(&b, "%d. **%s** · %s · `%s`\n\n", i+1, item.Source, item.Timestamp, item.Query)
			for _, line := range strings.Split(item.Excerpt, "\n") {
				fmt.Fprintf(&b, "   > %s\n", strings.TrimRight(line, "\r"))
			}
			b.WriteString("\n")
		}
	}

	if message != "" {
		fmt.Fprintf(&b, "### 交给事故指挥\n\n%s\n", message)
	}

	return b.String()
}

func (r TriageResult) Render() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	known := "未命中已知模式"
	if r.KnownPattern.Bool() {
		known = "命中已知模式"
	}
	return renderFinding("分诊结论", r.Severity+" · "+r.BlastRadius, "分级与影响面", known+"；"+r.BlastRadius, r.Result, r.Message, r.Evidence), nil
}

func (f LogFinding) Render() (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return renderFinding("日志分析结论", f.Pattern, "错误模式", f.Onset, f.Result, f.Message, f.Evidence), nil
}

func (f MetricFinding) Render() (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return renderFinding("指标分析结论", f.Metric, "指标", f.Onset, f.Result, f.Message, f.Evidence), nil
}

func (f ChangeFinding) Render() (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return renderFinding("变更分析结论", f.Change, "变更", f.Onset, f.Result, f.Message, f.Evidence), nil
}
