package flowtemplates_test

import (
	"context"
	"errors"
	"testing"

	"opsagi/pkg/database"
	"opsagi/pkg/flowtemplates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingSeeder struct {
	calls []database.SeedFlowTemplatesParams
	rows  int64
	err   error
}

func (s *recordingSeeder) SeedFlowTemplates(
	ctx context.Context, arg database.SeedFlowTemplatesParams,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s.err != nil {
		return 0, s.err
	}
	s.calls = append(s.calls, arg)

	return s.rows, nil
}

func TestSeed_SendsTheWholeCatalogueForTheUserAndReturnsWhatItInserted(t *testing.T) {
	db := &recordingSeeder{rows: 3}

	inserted, err := flowtemplates.Seed(context.Background(), db, 42)
	require.NoError(t, err)

	assert.Equal(t, int64(3), inserted)
	require.Len(t, db.calls, 1)
	assert.Equal(t, int64(42), db.calls[0].UserID)

	presets := flowtemplates.Shipped()
	titles := make([]string, 0, len(presets))
	texts := make([]string, 0, len(presets))
	for _, preset := range presets {
		titles = append(titles, preset.Title)
		texts = append(texts, preset.Text)
	}

	assert.Equal(t, titles, db.calls[0].Titles)
	assert.Equal(t, texts, db.calls[0].Texts, "titles and texts must stay index-aligned")
}

func TestSeed_ReportsTheDatabaseFailureAsTheSeedFailing(t *testing.T) {
	boom := errors.New("connection reset")
	db := &recordingSeeder{err: boom}

	inserted, err := flowtemplates.Seed(context.Background(), db, 7)

	assert.Zero(t, inserted)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "flow templates", "the log line says which seed failed")
}

func TestSeed_StopsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inserted, err := flowtemplates.Seed(ctx, &recordingSeeder{rows: 3}, 7)

	assert.Zero(t, inserted)
	assert.ErrorIs(t, err, context.Canceled)
}
