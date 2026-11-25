//go:build postgres

package database_test

import (
	"context"
	"sort"
	"testing"

	"opsagi/pkg/database"
)

func TestFlowTemplates_SeedFlowTemplates_InsertsTheCatalogueOnceAndNothingOnTheSecondCall(t *testing.T) {
	db := openSchema(t)
	uid := seedUser(t, db)
	q := database.New(db)
	ctx := context.Background()

	inserted, err := q.SeedFlowTemplates(ctx, database.SeedFlowTemplatesParams{
		UserID: uid,
		Titles: []string{"first", "second"},
		Texts:  []string{"body one", "body two"},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("seeded %d templates, want 2", inserted)
	}

	again, err := q.SeedFlowTemplates(ctx, database.SeedFlowTemplatesParams{
		UserID: uid,
		Titles: []string{"a later release"},
		Texts:  []string{"body three"},
	})
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if again != 0 {
		t.Fatalf("the second call inserted %d rows; a claimed user must get nothing more", again)
	}

	rows, err := q.GetFlowTemplatesByUserID(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	titles := make([]string, 0, len(rows))
	for _, row := range rows {
		titles = append(titles, row.Title)
	}
	sort.Strings(titles)

	if len(titles) != 2 || titles[0] != "first" || titles[1] != "second" {
		t.Fatalf("the user ended up with %v, want the two titles the first call carried", titles)
	}

	var claims int
	if err := db.QueryRow(
		`SELECT count(*) FROM flow_template_seeds WHERE user_id = $1`, uid,
	).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 1 {
		t.Fatalf("%d claim rows, want exactly 1", claims)
	}
}
