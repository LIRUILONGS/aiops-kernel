package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opsagi/pkg/config"
)

func promToolsFor(t *testing.T, handler http.Handler) (*promTools, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewPromTools(&config.Config{PrometheusURL: server.URL}), server
}

func TestProm_Query_SendsAWellFormedRangeRequest(t *testing.T) {
	var seen []string

	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path, r.URL.Query().Get("query"), r.URL.Query().Get("start"), r.URL.Query().Get("end"), r.URL.Query().Get("step"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))

	if _, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"node_load1","range":"10m","step":"30s"}`)); err != nil {
		t.Fatalf("HandleQuery() = %v", err)
	}

	if seen[0] != "/api/v1/query_range" {
		t.Errorf("path = %q, want /api/v1/query_range", seen[0])
	}
	if seen[1] != "node_load1" {
		t.Errorf("query = %q, want node_load1", seen[1])
	}
	if seen[4] != "30" {
		t.Errorf("step = %q, want 30 seconds", seen[4])
	}
	if seen[2] == "" || seen[3] == "" || seen[2] >= seen[3] {
		t.Errorf("start/end = %q/%q, want an increasing window", seen[2], seen[3])
	}
}

func TestProm_Query_RendersTheJumpNotTheWholeSeries(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"__name__":"node_load1","instance":"lab:9100"},"values":[[1000,"0.5"],[1030,"0.6"],[1060,"4.1"],[1090,"4.2"]]}
	]}}`

	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))

	out, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"node_load1","range":"1m","step":"30s"}`))
	if err != nil {
		t.Fatalf("HandleQuery() = %v", err)
	}

	for _, want := range []string{
		"node_load1{instance=\"lab:9100\"}",
		"点数 4",
		"首值 0.5",
		"末值 4.2",
		"最小 0.5",
		"最大 4.2",
		"最大跳变",
		"0.6 → 4.1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestProm_Query_ReportsWhatTheServerRefusedVerbatim(t *testing.T) {
	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error: unexpected end of input"}`))
	}))

	_, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"node_load1{"}`))
	if err == nil {
		t.Fatal("HandleQuery() = nil, want the server's own message")
	}
	if !strings.Contains(err.Error(), "parse error: unexpected end of input") {
		t.Fatalf("HandleQuery() = %q, want it to carry the server's message", err)
	}
}

func TestProm_Query_SaysWhenTheWindowIsEmpty(t *testing.T) {
	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))

	out, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"absent_metric"}`))
	if err != nil {
		t.Fatalf("HandleQuery() = %v", err)
	}
	if !strings.Contains(out, "窗口内没有数据") {
		t.Fatalf("an empty result must say so:\n%s", out)
	}
}

func TestProm_Query_RejectsABadWindowBeforeItAsksTheServer(t *testing.T) {
	requests := 0

	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))

	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{name: "no query", args: `{}`, wantErr: "query 为空"},
		{name: "a window longer than a day", args: `{"query":"up","range":"48h"}`, wantErr: "超出 1m~24h"},
		{name: "a range that is not a duration", args: `{"query":"up","range":"yesterday"}`, wantErr: "不是合法时长"},
		{name: "a step that is not a duration", args: `{"query":"up","step":"fast"}`, wantErr: "不是合法时长"},
		{name: "more points than the server will return", args: `{"query":"up","range":"24h","step":"1s"}`, wantErr: "超过上限"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(tt.args))
			if err == nil {
				t.Fatalf("HandleQuery() = nil, want an error mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("HandleQuery() = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}

	if requests != 0 {
		t.Fatalf("the server saw %d requests, want 0: an invalid window must be refused locally", requests)
	}
}

func TestProm_Query_UnavailableWithoutAnEndpoint(t *testing.T) {
	tools := NewPromTools(&config.Config{})
	if tools.IsAvailable() {
		t.Fatal("IsAvailable() = true with no PROMETHEUS_URL: a metrics tool with nowhere to query must not reach a model")
	}
}

func TestProm_Series_SortsPointsAndSkipsUnparseableValues(t *testing.T) {
	serie := promSerie{
		Metric: map[string]string{"__name__": "m"},
		Values: [][2]any{
			{float64(1060), "3"},
			{float64(1000), "1"},
			{float64(1030), "NaN"},
			{float64(1090), "2"},
		},
	}

	points := serie.points()
	if len(points) != 3 {
		t.Fatalf("points = %d, want 3: a NaN sample is not a point", len(points))
	}
	if points[0].value != 1 || points[2].value != 2 {
		t.Fatalf("points are not sorted by time: %+v", points)
	}
}

func TestProm_StructuredBlock_CarriesWhatAChartNeeds(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"__name__":"node_load1","instance":"lab:9100"},"values":[[1000,"0.5"],[1030,"0.6"],[1060,"4.1"]]}
	]}}`

	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))

	out, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"node_load1","range":"1m","step":"30s"}`))
	if err != nil {
		t.Fatalf("HandleQuery() = %v", err)
	}

	const marker = "### 结构化结果"
	if !strings.Contains(out, marker) {
		t.Fatalf("output carries no structured block:\n%s", out)
	}

	block := out[strings.Index(out, marker):]
	start := strings.Index(block, "```json\n")
	end := strings.LastIndex(block, "\n```")
	if start < 0 || end < 0 {
		t.Fatalf("the structured block is not fenced as json:\n%s", block)
	}

	var payload struct {
		End    int64  `json:"end"`
		Query  string `json:"query"`
		Range  string `json:"range"`
		Step   string `json:"step"`
		Series []struct {
			Name   string      `json:"name"`
			Points [][2]string `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal([]byte(block[start+len("```json\n"):end]), &payload); err != nil {
		t.Fatalf("the structured block is not valid JSON: %v\n%s", err, block)
	}

	if payload.Query != "node_load1" || payload.Range != "1m0s" || payload.Step != "30s" || payload.End == 0 {
		t.Errorf("payload = %+v, want the query, window, step and end to be carried", payload)
	}
	if len(payload.Series) != 1 {
		t.Fatalf("series = %d, want 1", len(payload.Series))
	}
	if payload.Series[0].Name != `node_load1{instance="lab:9100"}` {
		t.Errorf("series name = %q, want the full selector", payload.Series[0].Name)
	}
	if len(payload.Series[0].Points) != 3 {
		t.Fatalf("points = %d, want 3", len(payload.Series[0].Points))
	}
	if payload.Series[0].Points[0][0] != "1000" || payload.Series[0].Points[2][1] != "4.1" {
		t.Errorf("points = %v, want epoch seconds and plain values", payload.Series[0].Points)
	}
}

func TestProm_StructuredBlock_IsAbsentWhenThereIsNothingToDraw(t *testing.T) {
	tools, _ := promToolsFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))

	out, err := tools.HandleQuery(context.Background(), PromQueryToolName, json.RawMessage(`{"query":"absent"}`))
	if err != nil {
		t.Fatalf("HandleQuery() = %v", err)
	}
	if strings.Contains(out, "结构化结果") {
		t.Fatalf("an empty window must not ship an empty chart payload:\n%s", out)
	}
}

func TestProm_Downsample_KeepsBothEndsAndTheStride(t *testing.T) {
	points := make([]promPoint, 0, 500)
	for i := range 500 {
		points = append(points, promPoint{at: time.Unix(int64(i), 0), value: float64(i)})
	}

	kept := downsamplePoints(points, 120)

	if len(kept) != 120 {
		t.Fatalf("kept %d points, want 120", len(kept))
	}
	if kept[0].at.Unix() != 0 || kept[len(kept)-1].at.Unix() != 499 {
		t.Fatalf("kept first %d and last %d, want the whole window", kept[0].at.Unix(), kept[len(kept)-1].at.Unix())
	}
	for i := 1; i < len(kept); i++ {
		if kept[i].at.Unix() <= kept[i-1].at.Unix() {
			t.Fatalf("points are not strictly increasing at %d", i)
		}
	}
}

func TestProm_Downsample_LeavesASmallSeriesAlone(t *testing.T) {
	points := []promPoint{{at: time.Unix(0, 0), value: 1}, {at: time.Unix(1, 0), value: 2}}

	if kept := downsamplePoints(points, 120); len(kept) != 2 {
		t.Fatalf("kept %d points, want both", len(kept))
	}
}
