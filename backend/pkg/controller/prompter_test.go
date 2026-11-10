package controller

import (
	"context"
	"errors"
	"testing"

	"opsagi/pkg/database"
	"opsagi/pkg/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type prompterFakeQuerier struct {
	database.Querier

	prompts []database.Prompt
	err     error
}

func (f *prompterFakeQuerier) GetUserPrompts(context.Context, int64) ([]database.Prompt, error) {
	return f.prompts, f.err
}

func TestPrompter_NewUserPrompter_OverlaysTheUsersPromptsOnTheDefaults(t *testing.T) {
	dbErr := errors.New("db connection lost")
	defaults := templates.NewDefaultPrompter()

	for _, tc := range []struct {
		name        string
		prompts     []database.Prompt
		dbErr       error
		wantCustom  map[templates.PromptType]string
		wantDefault []templates.PromptType
	}{
		{
			name:        "no user prompts",
			wantDefault: []templates.PromptType{templates.PromptTypeGenerator, templates.PromptTypeAssistant},
		},
		{
			name:        "one override",
			prompts:     []database.Prompt{{Type: database.PromptTypeGenerator, Prompt: "custom generator"}},
			wantCustom:  map[templates.PromptType]string{templates.PromptTypeGenerator: "custom generator"},
			wantDefault: []templates.PromptType{templates.PromptTypeAssistant, templates.PromptTypeTriage},
		},
		{
			name: "several overrides",
			prompts: []database.Prompt{
				{Type: database.PromptTypeGenerator, Prompt: "custom generator"},
				{Type: database.PromptTypeReporter, Prompt: "custom reporter"},
				{Type: database.PromptTypeRefiner, Prompt: "custom refiner"},
			},
			wantCustom: map[templates.PromptType]string{
				templates.PromptTypeGenerator: "custom generator",
				templates.PromptTypeReporter:  "custom reporter",
				templates.PromptTypeRefiner:   "custom refiner",
			},
			wantDefault: []templates.PromptType{
				templates.PromptTypeAssistant, templates.PromptTypeTriage, templates.PromptTypeLogAnalyst,
			},
		},
		{
			name:        "an empty body, which would surface deep in agent rendering",
			prompts:     []database.Prompt{{Type: database.PromptTypeGenerator, Prompt: ""}},
			wantDefault: []templates.PromptType{templates.PromptTypeGenerator},
		},
		{
			name: "an override using a variable the backend no longer provides",
			prompts: []database.Prompt{
				{Type: database.PromptTypeSummarizer, Prompt: "Search with {{.GoogleToolName}}"},
				{Type: database.PromptTypeReporter, Prompt: "custom reporter"},
			},
			wantCustom:  map[templates.PromptType]string{templates.PromptTypeReporter: "custom reporter"},
			wantDefault: []templates.PromptType{templates.PromptTypeSummarizer},
		},
		{name: "a database that cannot be read", dbErr: dbErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompter, err := newUserPrompter(context.Background(), &prompterFakeQuerier{prompts: tc.prompts, err: tc.dbErr}, 42)

			if tc.dbErr != nil {
				assert.ErrorIs(t, err, tc.dbErr, "a session is not built on defaults the user did not choose")
				assert.Nil(t, prompter)

				return
			}
			require.NoError(t, err)
			for pt, want := range tc.wantCustom {
				got, err := prompter.GetTemplate(pt)
				require.NoError(t, err)
				assert.Equal(t, want, got, "prompt %s", pt)
			}
			for _, pt := range tc.wantDefault {
				got, err := prompter.GetTemplate(pt)
				require.NoError(t, err)
				want, err := defaults.GetTemplate(pt)
				require.NoError(t, err)
				assert.Equal(t, want, got, "prompt %s keeps its default", pt)
			}
		})
	}
}
