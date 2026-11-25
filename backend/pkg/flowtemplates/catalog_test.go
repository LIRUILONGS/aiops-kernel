package flowtemplates_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"opsagi/pkg/flowtemplates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The caps the template mutations enforce. A preset over them would be seeded and then
// refused on save, which is worse than not shipping it.
const (
	maxTitleRunes = 255
	maxTextRunes  = 65536
)

func TestCatalog_Shipped_CoversEveryDomainWithATemplateWithinTheMutationLimits(t *testing.T) {
	presets := flowtemplates.Shipped()
	require.NotEmpty(t, presets)

	domains := map[string]int{}
	titles := map[string]bool{}

	for _, preset := range presets {
		domains[preset.Domain]++
		assert.NotEmpty(t, strings.TrimSpace(preset.Title), "a preset has an empty title")
		assert.False(t, titles[preset.Title], "title %q ships twice", preset.Title)
		titles[preset.Title] = true

		assert.NotEmpty(t, strings.TrimSpace(preset.Text), "%s: empty text", preset.Title)
		assert.LessOrEqual(t, utf8.RuneCountInString(preset.Title), maxTitleRunes, "%s: title over the cap", preset.Title)
		assert.LessOrEqual(t, utf8.RuneCountInString(preset.Text), maxTextRunes, "%s: text over the cap", preset.Title)
	}

	// The six labels the UI groups the presets under; a preset in a seventh domain would
	// render under a heading the picker does not know.
	for _, domain := range []string{"告警诊断", "日志与指标", "变更复盘", "容量与性能", "Kubernetes", "数据库与中间件"} {
		assert.NotZero(t, domains[domain], "no shipped template covers %q", domain)
	}
	assert.Len(t, domains, 6, "a shipped template names a domain no other one uses")
}

// The probes the shipped prompts promise; a template that names none of them asks for a
// conclusion without saying where to look, which is the way a flow stalls.
var probes = []string{
	"ops_system", "ops_containers", "ops_logs", "ops_baseline", "ops_diff",
	"ops_exec", "ops_watch", "ops_trace", "prom_query", "k8s_get",
}

func TestCatalog_Shipped_EachTemplateNamesAProbeThatAnswersIt(t *testing.T) {
	for _, preset := range flowtemplates.Shipped() {
		assert.True(t,
			slices.ContainsFunc(probes, func(probe string) bool { return strings.Contains(preset.Text, probe) }),
			"%q asks for evidence without naming a probe", preset.Title)
	}
}

func TestCatalog_Shipped_HandsOutACopy(t *testing.T) {
	first := flowtemplates.Shipped()
	require.NotEmpty(t, first)

	first[0] = flowtemplates.Preset{Domain: "rewritten", Title: "rewritten", Text: "rewritten"}

	assert.NotEqual(t, "rewritten", flowtemplates.Shipped()[0].Title, "a caller rewrote the catalogue")
}
