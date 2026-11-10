// External package: these tests use pkg/templates/validator, which imports pkg/templates.
package templates_test

import (
	"reflect"
	"strings"
	"testing"

	"opsagi/pkg/templates"
	"opsagi/pkg/templates/validator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templatesShippedPrompts returns every Prompt of GetDefaultPrompts keyed by its field path.
func templatesShippedPrompts(t *testing.T) map[string]templates.Prompt {
	t.Helper()

	defaultPrompts, err := templates.GetDefaultPrompts()
	require.NoError(t, err)

	prompts := map[string]templates.Prompt{}
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		for i := range v.NumField() {
			field, name := v.Field(i), path+"."+v.Type().Field(i).Name
			if prompt, ok := field.Interface().(templates.Prompt); ok {
				prompts[name] = prompt
			} else if field.Kind() == reflect.Struct {
				walk(field, name)
			}
		}
	}
	walk(reflect.ValueOf(*defaultPrompts), "DefaultPrompts")

	return prompts
}

// templatesCaseName turns a prompt type into a subtest sentence.
func templatesCaseName(promptType templates.PromptType) string {
	return "the " + strings.ReplaceAll(string(promptType), "_", " ") + " prompt"
}

func TestTemplates_GetDefaultPrompts_DeclaresExactlyTheVariablesEachPromptUses(t *testing.T) {
	prompts := templatesShippedPrompts(t)

	agents, tools := 0, 0
	shipped := map[templates.PromptType]bool{}
	for name, prompt := range prompts {
		if strings.HasPrefix(name, "DefaultPrompts.AgentsPrompts.") {
			agents++
		} else {
			tools++
		}
		shipped[prompt.Type] = true

		t.Run(templatesCaseName(prompt.Type), func(t *testing.T) {
			require.NotEmpty(t, strings.TrimSpace(prompt.Template), "%s has no template", name)

			used, err := validator.ExtractTemplateVariables(prompt.Template)
			require.NoError(t, err, name)
			assert.ElementsMatch(t, prompt.Variables, used, "%s: declared variables against the ones the template uses", name)

			_, declared := templates.PromptVariables[prompt.Type]
			assert.True(t, declared, "%s is missing from PromptVariables", prompt.Type)
		})
	}

	assert.Equal(t, 17, agents, "agent prompts")
	assert.Equal(t, 9, tools, "tool prompts")
	for promptType := range templates.PromptVariables {
		assert.True(t, shipped[promptType], "PromptVariables holds %s, which no shipped prompt has", promptType)
	}
}

func TestTemplates_GetDefaultPrompts_EveryPromptRendersWithDummyData(t *testing.T) {
	dummyData := validator.CreateDummyTemplateData()

	for name, prompt := range templatesShippedPrompts(t) {
		t.Run(templatesCaseName(prompt.Type), func(t *testing.T) {
			_, err := templates.RenderPrompt(string(prompt.Type), prompt.Template, dummyData)
			assert.NoError(t, err, name)
		})
	}
}
