package opsremote

import (
	"strings"
	"testing"
)

const explainPlaceholderSyntaxError = `ERROR:  syntax error at or near "$1"
LINE 1: explain (format text, verbose off, generic_plan) select pid, ...
                         ^
`

const explainPlaceholderOperatorError = `ERROR:  operator is not unique: unknown - unknown
LINE 1: explain (format text, verbose off, generic_plan) select count(*) ...
                    ^
`

func TestAdvice_Advice_ExplainsAPlaceholderTheGenericPlanCannotType(t *testing.T) {
	t.Parallel()

	for name, output := range map[string]string{
		"a parameter inside a function call": explainPlaceholderSyntaxError,
		"an operator between two unknowns":   explainPlaceholderOperatorError,
	} {
		advice := Advice("pg_explain", output)
		if advice == "" {
			t.Fatalf("%s: no advice for %q", name, output)
		}

		for _, want := range []string{"GENERIC_PLAN", "占位符", "rank"} {
			if !strings.Contains(advice, want) {
				t.Errorf("%s: advice = %q, want it to mention %q", name, advice, want)
			}
		}
	}
}

func TestAdvice_Advice_StaysQuietForAPlanAndForOtherCommands(t *testing.T) {
	t.Parallel()

	plan := `Seq Scan on pgbench_accounts  (cost=0.00..145000.00 rows=1 width=8)
  Filter: (region = 'eu'::text)`

	if advice := Advice("pg_explain", plan); advice != "" {
		t.Errorf("advice = %q, want none for a plan", advice)
	}

	if advice := Advice("pg_stat_statements", explainPlaceholderSyntaxError); advice != "" {
		t.Errorf("advice = %q, want none outside pg_explain", advice)
	}
}
