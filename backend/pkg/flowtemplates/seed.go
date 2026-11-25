package flowtemplates

import (
	"context"
	"fmt"

	"opsagi/pkg/database"
)

// Seeder is the slice of the query layer the seed needs, narrow enough that a test
// can stand in for the database.
type Seeder interface {
	SeedFlowTemplates(ctx context.Context, arg database.SeedFlowTemplatesParams) (int64, error)
}

// Seed copies the shipped templates into the user's list the first time it is called
// for that user and inserts nothing afterwards. The claim row and the inserts are one
// statement, so a failed call leaves neither a partial list nor a claim on the user.
func Seed(ctx context.Context, db Seeder, userID int64) (int64, error) {
	presets := Shipped()
	titles := make([]string, 0, len(presets))
	texts := make([]string, 0, len(presets))
	for _, preset := range presets {
		titles = append(titles, preset.Title)
		texts = append(texts, preset.Text)
	}

	inserted, err := db.SeedFlowTemplates(ctx, database.SeedFlowTemplatesParams{
		UserID: userID,
		Titles: titles,
		Texts:  texts,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to seed the shipped flow templates: %w", err)
	}

	return inserted, nil
}
