package providers

import (
	"reflect"
	"strings"
	"testing"

	"opsagi/pkg/templates"
	"opsagi/pkg/tools"
)

// The analysts have exactly one exit, and the model learns its fields from the
// prompt, not from the schema alone. Two of these prompts had drifted into naming
// fields their struct never carried (summary, first_occurrence, patterns), so a
// delegation that ended the chain through the finding tool described a payload the
// tool would refuse - which is how four flows in a row closed on the commander's
// diagnosis instead of a specialist's finding.
func TestAIOpsHandlers_AnalystPromptsNameEveryFieldItsFindingCarries(t *testing.T) {
	prompts, err := templates.GetDefaultPrompts()
	if err != nil {
		t.Fatalf("default prompts: %v", err)
	}

	cases := []struct {
		role    string
		prompt  templates.Prompt
		finding any
	}{
		{"triage", prompts.AgentsPrompts.Triage.System, tools.TriageResult{}},
		{"log_analyst", prompts.AgentsPrompts.LogAnalyst.System, tools.LogFinding{}},
		{"metrics_analyst", prompts.AgentsPrompts.MetricsAnalyst.System, tools.MetricFinding{}},
		{"change_analyst", prompts.AgentsPrompts.ChangeAnalyst.System, tools.ChangeFinding{}},
	}

	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			typ := reflect.TypeOf(c.finding)
			for i := range typ.NumField() {
				name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
				if name == "" || name == "-" {
					continue
				}
				if !strings.Contains(c.prompt.Template, "- "+name+"：") {
					t.Errorf("prompt never names %q as a field of its finding; the tool requires it", name)
				}
			}
		})
	}
}
