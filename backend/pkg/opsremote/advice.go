package opsremote

import "strings"

// explainGenericPlanLimits are the two shapes PostgreSQL reports when a
// placeholder's type cannot be inferred: a parameter the parser will not accept
// in that position, and an operator it cannot resolve between unknown operands.
var explainGenericPlanLimits = []string{
	`syntax error at or near "$1"`,
	"operator is not unique",
}

// Advice turns a catalogue command's failure into one sentence the model can act
// on; the raw error reads as if the tool were broken. Empty means the output
// needs no explanation.
func Advice(commandID, stdout string) string {
	if commandID != "pg_explain" {
		return ""
	}

	for _, marker := range explainGenericPlanLimits {
		if strings.Contains(stdout, marker) {
			return "这条语句带占位符（$1、$2…），EXPLAIN 的 GENERIC_PLAN 推不出它们的类型，所以计划拿不到；换一个 rank 看别的语句，或者到应用侧用真实参数复现。"
		}
	}

	return ""
}
