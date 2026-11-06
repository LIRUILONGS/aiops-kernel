package tools

import (
	"encoding/json"
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

	report, err := diagnosis.Render()
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
