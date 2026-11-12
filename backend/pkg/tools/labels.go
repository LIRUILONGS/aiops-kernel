package tools

import "strings"

// reportLabels is the scaffolding a structured exit wraps its prose in: the
// headings, the field names and the few fixed phrases. The prose belongs to the
// model and follows the flow's language; the scaffolding has to follow it too,
// because a Chinese heading over English prose is what a mixed-language report
// looks like once it reaches the operator.
type reportLabels struct {
	Sep                 string
	ParenOpen           string
	ParenClose          string
	DiagnosisTitle      string
	OptimizationTitle   string
	RootCause           string
	Confidence          string
	BlastRadius         string
	NeedsHuman          string
	Yes                 string
	No                  string
	Evidence            string
	Recommendation      string
	Action              string
	Risk                string
	Rollback            string
	Structured          string
	Verdict             string
	Bottleneck          string
	Baseline            string
	Proposal            string
	ExpectedGain        string
	ObservedGain        string
	NotMeasured         string
	MeasuredAgainst     string
	MeasuredValue       string
	NotFilled           string
	TargetMet           string
	TargetMissed        string
	Onset               string
	Handover            string
	TriageTitle         string
	TriageHeadline      string
	TriageKnownPattern  string
	PatternHit          string
	PatternMiss         string
	LogTitle            string
	LogHeadline         string
	MetricTitle         string
	MetricHeadline      string
	ChangeTitle         string
	ChangeHeadline      string
}

var chineseLabels = reportLabels{
	Sep:                 "：",
	ParenOpen:           "（",
	ParenClose:          "）",
	DiagnosisTitle:      "## 诊断结论",
	OptimizationTitle:   "## 性能优化结论",
	RootCause:           "根因",
	Confidence:          "置信度",
	BlastRadius:         "影响面",
	NeedsHuman:          "需要人工介入",
	Yes:                 "是",
	No:                  "否",
	Evidence:            "证据",
	Recommendation:      "建议动作",
	Action:              "动作",
	Risk:                "风险",
	Rollback:            "回滚",
	Structured:          "结构化结果",
	Verdict:             "结论",
	Bottleneck:          "瓶颈",
	Baseline:            "变更前",
	Proposal:            "建议改动",
	ExpectedGain:        "预期收益",
	ObservedGain:        "实测收益",
	NotMeasured:         "未复测",
	MeasuredAgainst:     "对照目标",
	MeasuredValue:       "复测值",
	NotFilled:           "未填",
	TargetMet:           "达标",
	TargetMissed:        "未达标",
	Onset:               "时间点",
	Handover:            "交给事故指挥",
	TriageTitle:         "## 分诊结论",
	TriageHeadline:      "分级与影响面",
	TriageKnownPattern:  "已知模式",
	PatternHit:          "命中已知模式",
	PatternMiss:         "未命中已知模式",
	LogTitle:            "## 日志分析结论",
	LogHeadline:         "错误模式",
	MetricTitle:         "## 指标分析结论",
	MetricHeadline:      "指标",
	ChangeTitle:         "## 变更分析结论",
	ChangeHeadline:      "变更",
}

var englishLabels = reportLabels{
	Sep:                 ": ",
	ParenOpen:           "(",
	ParenClose:          ")",
	DiagnosisTitle:      "## Diagnosis",
	OptimizationTitle:   "## Optimisation result",
	RootCause:           "Root cause",
	Confidence:          "Confidence",
	BlastRadius:         "Blast radius",
	NeedsHuman:          "Needs a human",
	Yes:                 "yes",
	No:                  "no",
	Evidence:            "Evidence",
	Recommendation:      "Recommendation",
	Action:              "Action",
	Risk:                "Risk",
	Rollback:            "Rollback",
	Structured:          "Structured result",
	Verdict:             "Verdict",
	Bottleneck:          "Bottleneck",
	Baseline:            "Before the change",
	Proposal:            "Proposed change",
	ExpectedGain:        "Expected gain",
	ObservedGain:        "Measured gain",
	NotMeasured:         "not re-measured",
	MeasuredAgainst:     "Compared against",
	MeasuredValue:       "Measured value",
	NotFilled:           "not given",
	TargetMet:           "met",
	TargetMissed:        "missed",
	Onset:               "Onset",
	Handover:            "Handover to the commander",
	TriageTitle:         "## Triage",
	TriageHeadline:      "Severity and blast radius",
	TriageKnownPattern:  "Known pattern",
	PatternHit:          "known pattern hit",
	PatternMiss:         "no known pattern",
	LogTitle:            "## Log finding",
	LogHeadline:         "Error pattern",
	MetricTitle:         "## Metric finding",
	MetricHeadline:      "Metric",
	ChangeTitle:         "## Change finding",
	ChangeHeadline:      "Change",
}

// LabelsFor picks the scaffolding set for a flow language. Languages arrive as
// English names from the chooser ("Chinese", "English", "Russian"); anything
// that is not Chinese reads in English rather than in a language the operator
// may not have.
func LabelsFor(lang string) reportLabels {
	if isChinese(lang) {
		return chineseLabels
	}

	return englishLabels
}

// planLabels are the two hints PlanDescription appends to a plan entry. They sit
// apart from reportLabels because a plan entry is read by the executing chain,
// not by the reporter.
type planLabels struct {
	Sep            string
	ExpectedSignal string
	AssignedTo     string
}

var (
	chinesePlanLabels = planLabels{Sep: "：", ExpectedSignal: "期望信号", AssignedTo: "建议执行者"}
	englishPlanLabels = planLabels{Sep: ": ", ExpectedSignal: "Expected signal", AssignedTo: "Suggested owner"}
)

func PlanLabelsFor(lang string) planLabels {
	if isChinese(lang) {
		return chinesePlanLabels
	}

	return englishPlanLabels
}

func isChinese(lang string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(lang))

	return strings.HasPrefix(trimmed, "chinese") || strings.HasPrefix(trimmed, "zh") || strings.Contains(lang, "中文")
}
