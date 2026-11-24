package tools

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func validDiagnosis() Diagnosis {
	confidence := 0.86
	needsHuman := Bool(true)

	return Diagnosis{
		RootCause: "opslab-broken 的 nginx 配置里有非法指令 unknown_directive_from_opslab，进程启动即退出，被 restart: always 拖成崩溃重启循环",
		Evidence: []EvidenceItem{
			{
				Source:    "containers",
				Query:     "ops_containers",
				Timestamp: "2026-11-06T13:40:12Z",
				Excerpt:   "opslab-broken  restarting  10  1  nginx:latest",
			},
			{
				Source:    "logs",
				Query:     "ops_logs source=opslab-broken lines=200",
				Timestamp: "2026-11-06T13:40:14Z",
				Excerpt:   `nginx: [emerg] unknown directive "unknown_directive_from_opslab" in /etc/nginx/nginx.conf:10`,
			},
		},
		Confidence:  &confidence,
		BlastRadius: "lab 主机上由 opslab-broken 提供的 HTTP 服务完全不可用",
		Recommendation: DiagnosisRecommendation{
			Action:   "删除 /etc/nginx/nginx.conf 第 10 行的非法指令并重建容器",
			Risk:     "low",
			Rollback: "把 nginx.conf 换回上一版并重建容器",
		},
		NeedsHuman: &needsHuman,
	}
}

func TestAIOps_Diagnosis_UnmarshalAcceptsAToolCallPayload(t *testing.T) {
	payload := `{
		"root_cause": "配置错误",
		"evidence": [
			{"source":"containers","query":"ops_containers","timestamp":"2026-11-06T13:40:12Z","excerpt":"restarting 10"},
			{"source":"logs","query":"ops_logs","timestamp":"2026-11-06T13:40:14Z","excerpt":"[emerg] unknown directive"}
		],
		"confidence": 0.5,
		"blast_radius": "单机",
		"recommendation": {"action":"无需动作","risk":"unknown","rollback":"无需撤销"},
		"needs_human": false
	}`

	var parsed Diagnosis
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("a complete payload must validate, got %v", err)
	}
	if len(parsed.Evidence) != 2 {
		t.Fatalf("evidence parsed as %d items, want 2", len(parsed.Evidence))
	}
	if parsed.NeedsHuman == nil || parsed.NeedsHuman.Bool() {
		t.Fatalf("needs_human=false must arrive as a present false, got %v", parsed.NeedsHuman)
	}
}

func TestAIOps_Diagnosis_ValidateRefusesAnIncompleteConclusion(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Diagnosis)
		wantErr string
	}{
		{
			name:    "no root cause",
			mutate:  func(d *Diagnosis) { d.RootCause = " " },
			wantErr: "root_cause 为空",
		},
		{
			name:    "no evidence",
			mutate:  func(d *Diagnosis) { d.Evidence = nil },
			wantErr: "至少 2 条",
		},
		{
			name:    "one evidence item",
			mutate:  func(d *Diagnosis) { d.Evidence = d.Evidence[:1] },
			wantErr: "evidence 只有 1 条",
		},
		{
			name:    "source outside the set",
			mutate:  func(d *Diagnosis) { d.Evidence[0].Source = "prometheus" },
			wantErr: "evidence[0].source",
		},
		{
			name:    "evidence without query",
			mutate:  func(d *Diagnosis) { d.Evidence[1].Query = "" },
			wantErr: "evidence[1].query 为空",
		},
		{
			name:    "evidence without timestamp",
			mutate:  func(d *Diagnosis) { d.Evidence[1].Timestamp = "" },
			wantErr: "evidence[1].timestamp 为空",
		},
		{
			name:    "evidence without excerpt",
			mutate:  func(d *Diagnosis) { d.Evidence[1].Excerpt = "" },
			wantErr: "evidence[1].excerpt 为空",
		},
		{
			name:    "confidence omitted",
			mutate:  func(d *Diagnosis) { d.Confidence = nil },
			wantErr: "confidence 缺失",
		},
		{
			name: "confidence above one",
			mutate: func(d *Diagnosis) {
				high := 1.4
				d.Confidence = &high
			},
			wantErr: "超出 0 到 1",
		},
		{
			name:    "blast radius omitted",
			mutate:  func(d *Diagnosis) { d.BlastRadius = "" },
			wantErr: "blast_radius 为空",
		},
		{
			name:    "action omitted",
			mutate:  func(d *Diagnosis) { d.Recommendation.Action = "" },
			wantErr: "recommendation.action 为空",
		},
		{
			name:    "risk outside the set",
			mutate:  func(d *Diagnosis) { d.Recommendation.Risk = "severe" },
			wantErr: "recommendation.risk",
		},
		{
			name:    "rollback omitted",
			mutate:  func(d *Diagnosis) { d.Recommendation.Rollback = "" },
			wantErr: "recommendation.rollback 为空",
		},
		{
			name: "evidence described instead of cited",
			mutate: func(d *Diagnosis) {
				d.Evidence = []EvidenceItem{
					{Source: "logs", Query: "查了 opslab-broken 的容器日志", Timestamp: "13:40", Excerpt: "PANIC"},
					{Source: "containers", Query: "ops_containers", Timestamp: "13:40", Excerpt: "restarts 4908"},
				}
			},
			wantErr: "没有点名工具",
		},
		{
			name:    "needs human omitted",
			mutate:  func(d *Diagnosis) { d.NeedsHuman = nil },
			wantErr: "needs_human 缺失",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnosis := validDiagnosis()
			tt.mutate(&diagnosis)

			err := diagnosis.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: unexpected error %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate accepted an incomplete diagnosis")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not name the field; want %q", err, tt.wantErr)
			}
		})
	}
}

func TestAIOps_Diagnosis_RenderKeepsTheEvidenceVerbatim(t *testing.T) {
	diagnosis := validDiagnosis()

	report, err := diagnosis.Render("Chinese")
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{
		"## 诊断结论",
		`nginx: [emerg] unknown directive "unknown_directive_from_opslab"`,
		"ops_logs source=opslab-broken lines=200",
		"**需要人工介入**：是",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report lost %q\n%s", want, report)
		}
	}

	var embedded Diagnosis
	start := strings.Index(report, "```json\n") + len("```json\n")
	end := strings.Index(report[start:], "\n```")
	if start < len("```json\n") || end < 0 {
		t.Fatalf("report has no json block:\n%s", report)
	}
	if err := json.Unmarshal([]byte(report[start:start+end]), &embedded); err != nil {
		t.Fatalf("embedded json does not parse back: %v", err)
	}
	if embedded.RootCause != diagnosis.RootCause || len(embedded.Evidence) != len(diagnosis.Evidence) {
		t.Fatalf("embedded json lost fields: %+v", embedded)
	}
}

// The scaffolding used to be hardcoded Chinese while the body followed the
// flow's language, so an English flow reached the operator with Chinese headings
// over English prose.
func TestAIOps_Renderer_FollowsTheFlowLanguage(t *testing.T) {
	evidence := []EvidenceItem{{
		Source:    "logs",
		Query:     "ops_logs source=opslab-broken",
		Timestamp: "2026-11-06T13:40:14Z",
		Excerpt:   "[emerg] unknown directive",
	}}
	knownPattern := Bool(true)

	tests := []struct {
		name   string
		render func(lang string) (string, error)
		wantEN []string
		wantZH []string
	}{
		{
			name:   "diagnosis",
			render: validDiagnosis().Render,
			wantEN: []string{"## Diagnosis", "**Root cause**: ", "**Confidence**: 0.86", "### Evidence", "### Recommendation", "### Structured result"},
			wantZH: []string{"## 诊断结论", "**根因**：", "**置信度**：0.86", "### 证据", "### 建议动作", "### 结构化结果"},
		},
		{
			name:   "optimisation",
			render: validOptimization().Render,
			wantEN: []string{"## Optimisation result", "**Verdict**: improved", "**Before the change**: ", "**Measured gain**: "},
			wantZH: []string{"## 性能优化结论", "**结论**：improved", "**变更前**：", "**实测收益**："},
		},
		{
			name: "triage",
			render: (TriageResult{
				Severity: "high", BlastRadius: "single host", KnownPattern: &knownPattern,
				Result: "report", Evidence: evidence,
			}).Render,
			wantEN: []string{"## Triage", "**Severity and blast radius**: high", "**Known pattern**: known pattern hit"},
			wantZH: []string{"## 分诊结论", "**分级与影响面**：high", "**已知模式**：命中已知模式"},
		},
		{
			name: "log finding",
			render: (LogFinding{
				Pattern: "emerg", Onset: "2026-11-06T13:40:14Z",
				Result: "report", Evidence: evidence,
			}).Render,
			wantEN: []string{"## Log finding", "**Error pattern**: emerg", "**Onset**: 2026-11-06T13:40:14Z"},
			wantZH: []string{"## 日志分析结论", "**错误模式**：emerg", "**时间点**：2026-11-06T13:40:14Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			english, err := tt.render("English")
			if err != nil {
				t.Fatalf("render in English: %v", err)
			}
			for _, want := range tt.wantEN {
				if !strings.Contains(english, want) {
					t.Errorf("English report is missing %q:\n%s", want, english)
				}
			}
			for _, zh := range tt.wantZH {
				if strings.Contains(english, zh) {
					t.Errorf("English report carries the Chinese label %q:\n%s", zh, english)
				}
			}

			chinese, err := tt.render("Chinese")
			if err != nil {
				t.Fatalf("render in Chinese: %v", err)
			}
			for _, want := range tt.wantZH {
				if !strings.Contains(chinese, want) {
					t.Errorf("Chinese report is missing %q:\n%s", want, chinese)
				}
			}
		})
	}
}

// A language the chooser can return but this product has no scaffolding for
// reads in English: half-translated labels would be worse than one language.
func TestAIOps_LabelsFor_DefaultsToEnglishForAnUnknownLanguage(t *testing.T) {
	for _, lang := range []string{"Russian", "English", "", "  "} {
		if got := LabelsFor(lang); got != englishLabels {
			t.Errorf("LabelsFor(%q) did not pick the English set", lang)
		}
	}
	for _, lang := range []string{"Chinese", "chinese", "Chinese (Simplified)", "中文"} {
		if got := LabelsFor(lang); got != chineseLabels {
			t.Errorf("LabelsFor(%q) did not pick the Chinese set", lang)
		}
	}
}

func TestAIOps_Findings_ValidateRefusesAnEmptyReport(t *testing.T) {
	evidence := []EvidenceItem{{
		Source:    "logs",
		Query:     "ops_logs source=opslab-broken",
		Timestamp: "2026-11-06T13:40:14Z",
		Excerpt:   "[emerg] unknown directive",
	}}
	knownPattern := Bool(true)

	tests := []struct {
		name    string
		finding interface{ Validate() error }
		wantErr string
	}{
		{
			name:    "triage without severity",
			finding: TriageResult{BlastRadius: "单机", KnownPattern: &knownPattern, Result: "报告", Evidence: evidence},
			wantErr: "severity",
		},
		{
			name:    "triage with an unknown severity",
			finding: TriageResult{Severity: "urgent", BlastRadius: "单机", KnownPattern: &knownPattern, Result: "报告", Evidence: evidence},
			wantErr: "severity=\"urgent\"",
		},
		{
			name:    "triage without known_pattern",
			finding: TriageResult{Severity: "high", BlastRadius: "单机", Result: "报告", Evidence: evidence},
			wantErr: "known_pattern 缺失",
		},
		{
			name:    "triage without a report",
			finding: TriageResult{Severity: "high", BlastRadius: "单机", KnownPattern: &knownPattern, Evidence: evidence},
			wantErr: "result 为空",
		},
		{
			name:    "triage without evidence",
			finding: TriageResult{Severity: "high", BlastRadius: "单机", KnownPattern: &knownPattern, Result: "报告"},
			wantErr: "evidence 只有 0 条",
		},
		{
			name:    "log finding without a pattern",
			finding: LogFinding{Onset: "13:40", Result: "报告", Evidence: evidence},
			wantErr: "pattern 为空",
		},
		{
			name:    "metric finding without an onset",
			finding: MetricFinding{Metric: "container_memory_working_set_bytes", Result: "报告", Evidence: evidence},
			wantErr: "onset 为空",
		},
		{
			name:    "change finding without a change",
			finding: ChangeFinding{Onset: "13:40", Result: "报告", Evidence: evidence},
			wantErr: "change 为空",
		},
		{
			name:    "triage that is complete",
			finding: TriageResult{Severity: "high", BlastRadius: "单机", KnownPattern: &knownPattern, Result: "报告", Evidence: evidence},
		},
		{
			name:    "log finding that is complete",
			finding: LogFinding{Pattern: "unknown directive", Onset: "13:40", Result: "报告", Evidence: evidence},
		},
		{
			name: "a log finding whose evidence comes from the database, which the log analyst cannot open",
			finding: LogFinding{
				Pattern: "unknown directive", Onset: "13:40", Result: "报告",
				Evidence: []EvidenceItem{{
					Source: "containers", Query: "ops_exec command=pg_locks container=opslab-db database=lockdb",
					Timestamp: "13:40", Excerpt: "1 个会话在等锁",
				}},
			},
			wantErr: "工具表里没有它",
		},
		{
			name: "a log finding whose evidence names no tool at all",
			finding: LogFinding{
				Pattern: "unknown directive", Onset: "13:40", Result: "报告",
				Evidence: []EvidenceItem{{
					Source: "logs", Query: "查了 opslab-broken 的容器日志",
					Timestamp: "13:40", Excerpt: "[emerg] unknown directive",
				}},
			},
			wantErr: "没有点名工具",
		},
		{
			name: "a metric finding whose evidence comes from a database the metrics analyst cannot open",
			finding: MetricFinding{
				Metric: "container_memory_working_set_bytes", Onset: "13:40", Result: "报告",
				Evidence: []EvidenceItem{{
					Source: "metrics", Query: "ops_exec command=pg_stat_activity container=opslab-db database=bench",
					Timestamp: "13:40", Excerpt: "0 个长事务",
				}},
			},
			wantErr: "工具表里没有它",
		},
		{
			name: "a change finding whose evidence comes from ops_exec, which the change analyst does hold",
			finding: ChangeFinding{
				Change: "16:02 改了 max_connections", Onset: "16:02", Result: "报告",
				Evidence: []EvidenceItem{{
					Source: "host", Query: "ops_exec command=pg_lock_wait_summary container=opslab-db database=lockdb",
					Timestamp: "16:02", Excerpt: "0|0|0",
				}},
			},
		},
		{
			name: "a triage whose evidence names two tools, both of them its own",
			finding: TriageResult{
				Severity: "high", BlastRadius: "单机", KnownPattern: &knownPattern, Result: "报告",
				Evidence: []EvidenceItem{
					{Source: "containers", Query: "ops_containers filter=opslab-broken", Timestamp: "13:40", Excerpt: "restarts 4908"},
					{Source: "logs", Query: "ops_logs source=opslab-broken", Timestamp: "13:40", Excerpt: "PANIC"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.finding.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: unexpected error %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate accepted an incomplete finding")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not name the field; want %q", err, tt.wantErr)
			}
		})
	}
}

func TestAIOps_Registry_TheFiveExitToolsAreReachable(t *testing.T) {
	tests := []struct {
		name     string
		wantType ToolType
		required []string
	}{
		{DiagnosisToolName, BarrierToolType, []string{"root_cause", "evidence", "blast_radius", "recommendation"}},
		{TriageResultToolName, StoreAgentResultToolType, []string{"severity", "blast_radius", "result", "evidence"}},
		{LogFindingToolName, StoreAgentResultToolType, []string{"pattern", "onset", "result", "evidence"}},
		{MetricFindingToolName, StoreAgentResultToolType, []string{"metric", "onset", "result", "evidence"}},
		{ChangeFindingToolName, StoreAgentResultToolType, []string{"change", "onset", "result", "evidence"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetToolType(tt.name); got != tt.wantType {
				t.Fatalf("tool type = %s, want %s", got, tt.wantType)
			}

			definition, ok := registryDefinitions[tt.name]
			if !ok {
				t.Fatalf("%s has no registry definition", tt.name)
			}
			if definition.Description == "" {
				t.Fatalf("%s has no description: the model cannot know when to call it", tt.name)
			}

			raw, err := json.Marshal(definition.Parameters)
			if err != nil {
				t.Fatalf("marshal schema: %v", err)
			}
			var schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatalf("unmarshal schema: %v", err)
			}

			present := make(map[string]bool, len(schema.Required))
			for _, field := range schema.Required {
				present[field] = true
			}
			for _, field := range tt.required {
				if !present[field] {
					t.Fatalf("%s schema does not require %q; required are %v", tt.name, field, schema.Required)
				}
			}
		})
	}
}

func TestAIOps_Diagnosis_EvidenceSchemaNamesItsFourFields(t *testing.T) {
	definition, ok := registryDefinitions[DiagnosisToolName]
	if !ok {
		t.Fatalf("diagnosis has no registry definition")
	}

	raw, err := json.Marshal(definition.Parameters)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}

	var schema struct {
		Properties struct {
			Evidence struct {
				Type  string `json:"type"`
				Items struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Description string `json:"description"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"evidence"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	if schema.Properties.Evidence.Type != "array" {
		t.Fatalf("evidence is %q; an array is what forces the model to enumerate", schema.Properties.Evidence.Type)
	}
	for _, field := range []string{"source", "query", "timestamp", "excerpt"} {
		if _, ok := schema.Properties.Evidence.Items.Properties[field]; !ok {
			t.Fatalf("evidence item has no %q field", field)
		}
		if schema.Properties.Evidence.Items.Properties[field].Description == "" {
			t.Fatalf("evidence item %q has no description", field)
		}
	}
}

func validOptimization() OptimizationResult {
	confidence := 0.7

	return OptimizationResult{
		Bottleneck:   "opslab-slow 的单请求串行处理把 CPU 压在 3% 但延迟 2s：瓶颈是 sleep 而不是算力",
		Baseline:     "ops_baseline 2026-11-06T18:20:00+08:00：负载 0.00/0.02/0.04，CPU 3%，内存 31%，被压测的容器 opslab-slow 平均响应 2043ms",
		Proposal:     "把 opslab-slow 的 WORKER_COUNT 从 1 改为 4，改完重建容器再压一轮",
		ExpectedGain: "吞吐提升到约 4 倍，P95 从 2.1s 降到 0.6s 以内",
		ObservedGain: "ops_diff 显示平均响应 2043ms -> 559ms，负载 0.04 -> 0.09",
		Verdict:      "improved",
		Rollback:     "把 WORKER_COUNT 改回 1 并重建容器",
		Evidence: []EvidenceItem{
			{Source: "host", Query: "ops_baseline", Timestamp: "2026-11-06T18:20:00+08:00", Excerpt: "load 0.00 0.02 0.04, cpu 3%"},
			{Source: "metrics", Query: "ops_diff", Timestamp: "2026-11-06T18:41:00+08:00", Excerpt: "avg_latency_ms 2043 -> 559 (improved)"},
		},
		Confidence: &confidence,
	}
}

func TestAIOps_Optimization_RefusesAConclusionThatWasNeverMeasured(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*OptimizationResult)
		wantErr string
	}{
		{
			name:   "a measured improvement",
			mutate: func(*OptimizationResult) {},
		},
		{
			name:    "no bottleneck",
			mutate:  func(o *OptimizationResult) { o.Bottleneck = "" },
			wantErr: "bottleneck 为空",
		},
		{
			name:    "no baseline",
			mutate:  func(o *OptimizationResult) { o.Baseline = "" },
			wantErr: "baseline 为空",
		},
		{
			name:    "no rollback",
			mutate:  func(o *OptimizationResult) { o.Rollback = "" },
			wantErr: "rollback 为空",
		},
		{
			name:    "an unknown verdict",
			mutate:  func(o *OptimizationResult) { o.Verdict = "faster" },
			wantErr: "verdict",
		},
		{
			name: "improved without a measurement",
			mutate: func(o *OptimizationResult) {
				o.ObservedGain = ""
			},
			wantErr: "没有复测就不能算改善",
		},
		{
			name: "no measurable effect with no observed value",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "no_measurable_effect"
				o.ObservedGain = ""
			},
			wantErr: "表示已经复测过",
		},
		{
			name: "a measured verdict that carries no numbers",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "degraded"
				o.ObservedGain = ""
			},
			wantErr: "verdict=degraded",
		},
		{
			name: "degraded with its measurement",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "degraded"
			},
		},
		{
			name: "no measurable effect with its measurement",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "no_measurable_effect"
			},
		},
		{
			name: "unverified carrying a measured value",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "unverified"
			},
			wantErr: "verdict=unverified 但 observed_gain 有内容",
		},
		{
			name:    "no confidence",
			mutate:  func(o *OptimizationResult) { o.Confidence = nil },
			wantErr: "confidence 缺失",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validOptimization()
			tc.mutate(&result)

			err := result.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused a valid optimisation: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted an optimisation that cannot be verified")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("refusal %q does not say %q", err, tc.wantErr)
			}
		})
	}
}

func TestAIOps_Optimization_RenderLeadsWithTheVerdict(t *testing.T) {
	report, err := validOptimization().Render("Chinese")
	if err != nil {
		t.Fatalf("Render() refused a valid optimisation: %v", err)
	}

	for _, want := range []string{"## 性能优化结论", "**结论**：improved", "**变更前**：", "**实测收益**：ops_diff 显示平均响应", "```json"} {
		if !strings.Contains(report, want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}

	unverified := validOptimization()
	unverified.Verdict = "unverified"
	unverified.ObservedGain = ""

	report, err = unverified.Render("Chinese")
	if err != nil {
		t.Fatalf("Render() refused an unverified optimisation: %v", err)
	}
	if !strings.Contains(report, "**实测收益**：未复测") {
		t.Errorf("an unverified optimisation must say it was not re-measured:\n%s", report)
	}
}

// The block a user can type by hand, and the one the web form appends, are the
// same shape: a marker line and three labelled lines.
func TestAIOps_PerformanceGoal_ParsesTheBlockTheFormAppends(t *testing.T) {
	input := "把 caddy 容器的 p99 延迟降下来\n\n【性能目标】\n- 指标：p99 延迟\n- 目标：< 200ms\n- 约束：不要重启 caddy"

	goal := ParsePerformanceGoal(input)

	if goal.Metric != "p99 延迟" {
		t.Fatalf("metric = %q, want %q", goal.Metric, "p99 延迟")
	}
	if goal.Target != "< 200ms" {
		t.Fatalf("target = %q, want %q", goal.Target, "< 200ms")
	}
	if goal.Constraint != "不要重启 caddy" {
		t.Fatalf("constraint = %q, want %q", goal.Constraint, "不要重启 caddy")
	}
}

func TestAIOps_PerformanceGoal_ReadsWhatIsThereAndNothingElse(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wanted PerformanceGoal
	}{
		{
			name:   "no block at all",
			input:  "看看这台机器为什么慢",
			wanted: PerformanceGoal{},
		},
		{
			name:   "only a metric",
			input:  "降延迟\n\n【性能目标】\n- 指标：p99",
			wanted: PerformanceGoal{Metric: "p99"},
		},
		{
			name:   "ascii colons and no bullets",
			input:  "【性能目标】\n指标: p99 延迟\n目标: < 200ms",
			wanted: PerformanceGoal{Metric: "p99 延迟", Target: "< 200ms", Op: "<", Value: floatPtr(200)},
		},
		{
			name:   "a following section is not part of the goal",
			input:  "【性能目标】\n- 指标：p99\n\n【禁止事项】\n- 目标：不要动证书",
			wanted: PerformanceGoal{Metric: "p99"},
		},
		{
			name:   "the block sits above the message",
			input:  "【性能目标】\n- 指标：p99\n\n把延迟降下来",
			wanted: PerformanceGoal{Metric: "p99"},
		},
		{
			name:   "a comparison in the target becomes a number to check",
			input:  "【性能目标】\n- 指标：p99 延迟\n- 目标：< 200ms",
			wanted: PerformanceGoal{Metric: "p99 延迟", Target: "< 200ms", Op: "<", Value: floatPtr(200)},
		},
		{
			name:   "a worded bound is the same bound",
			input:  "【性能目标】\n- 约束：重启次数 不超过 10 次",
			wanted: PerformanceGoal{Constraint: "重启次数 不超过 10 次", Op: "<=", Value: floatPtr(10)},
		},
		{
			name:  "the constraint supplies the number when the target has none",
			input: "【性能目标】\n- 指标：错误率\n- 约束：高于 0 就算回归",
			wanted: PerformanceGoal{
				Metric: "错误率", Constraint: "高于 0 就算回归", Op: ">", Value: floatPtr(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePerformanceGoal(tt.input)
			if !reflect.DeepEqual(got, tt.wanted) {
				t.Fatalf("ParsePerformanceGoal() = %+v, want %+v", got, tt.wanted)
			}
		})
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func TestAIOps_PerformanceGoal_RefusesAConclusionThatIgnoresTheDeclaredGoal(t *testing.T) {
	goal := PerformanceGoal{Metric: "p99 延迟", Target: "< 200ms"}

	tests := []struct {
		name    string
		mutate  func(*OptimizationResult)
		wantErr string
	}{
		{
			name: "a measurement that answers the goal",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
			},
		},
		{
			name: "a measured verdict that never names the goal",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = ""
			},
			wantErr: "声明了性能目标",
		},
		{
			name: "a measurement of some other metric",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "磁盘 IO < 30%"
			},
			wantErr: "没有提到这次 flow 要看的指标",
		},
		{
			name: "a goal answered without a before/after comparison",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.Evidence = []EvidenceItem{
					{Source: "host", Query: "ops_baseline", Timestamp: "2026-11-06T18:20:00+08:00", Excerpt: "load 0.00"},
					{Source: "host", Query: "ops_baseline after change", Timestamp: "2026-11-06T18:41:00+08:00", Excerpt: "load 0.09"},
				}
			},
			wantErr: "ops_diff",
		},
		{
			name: "an honest refusal to measure is allowed under a goal",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "unverified"
				o.ObservedGain = ""
				o.MeasuredAgainst = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validOptimization()
			tt.mutate(&result)

			err := result.ValidateAgainst(goal)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateAgainst() = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("ValidateAgainst() = nil, want an error mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateAgainst() = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestAIOps_PerformanceGoal_ChecksTheNumericTargetArithmetically(t *testing.T) {
	goal := PerformanceGoal{Metric: "p99 延迟", Target: "< 200ms", Op: "<", Value: floatPtr(200)}
	met := Bool(true)
	notMet := Bool(false)
	blank := Bool(false)

	tests := []struct {
		name    string
		mutate  func(*OptimizationResult)
		wantErr string
	}{
		{
			name: "a measurement that clears the target",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.MeasuredValue = floatPtr(140)
				o.TargetMet = &met
			},
		},
		{
			name: "a measurement that misses the target, reported honestly",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "degraded"
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.MeasuredValue = floatPtr(260)
				o.TargetMet = &notMet
			},
		},
		{
			name: "the boundary belongs to the operator",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.MeasuredValue = floatPtr(200)
				o.TargetMet = &blank
			},
		},
		{
			name: "a claim of meeting a target the number misses",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.MeasuredValue = floatPtr(260)
				o.TargetMet = &met
			},
			wantErr: "与算术不一致",
		},
		{
			name: "a claim of missing a target the number clears",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
				o.MeasuredValue = floatPtr(90)
				o.TargetMet = &notMet
			},
			wantErr: "与算术不一致",
		},
		{
			name: "a numeric target answered without a number",
			mutate: func(o *OptimizationResult) {
				o.MeasuredAgainst = "p99 延迟 < 200ms"
			},
			wantErr: "必须填 measured_value",
		},
		{
			name: "an honest refusal to measure still needs no number",
			mutate: func(o *OptimizationResult) {
				o.Verdict = "unverified"
				o.ObservedGain = ""
				o.MeasuredAgainst = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validOptimization()
			tt.mutate(&result)

			err := result.ValidateAgainst(goal)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateAgainst() = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("ValidateAgainst() = nil, want an error mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateAgainst() = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestAIOps_PerformanceGoal_WithoutAGoalNothingIsChecked(t *testing.T) {
	result := validOptimization()
	result.MeasuredAgainst = ""
	result.Evidence = []EvidenceItem{
		{Source: "host", Query: "ops_baseline", Timestamp: "2026-11-06T18:20:00+08:00", Excerpt: "load 0.00"},
		{Source: "metrics", Query: "ops_containers", Timestamp: "2026-11-06T18:41:00+08:00", Excerpt: "restarts 3"},
	}

	if err := result.ValidateAgainst(PerformanceGoal{}); err != nil {
		t.Fatalf("ValidateAgainst() = %v, want nil for a flow without a goal", err)
	}
}
