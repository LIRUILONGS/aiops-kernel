package controller

import (
	"testing"

	"opsagi/pkg/database"
)

func TestClaimPlanned_TakesEachSubtasksOnceAndNoMoreThanTheLimit(t *testing.T) {
	planned := []database.Subtask{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	claimed := map[int64]struct{}{2: {}}

	first := claimPlanned(planned, claimed, 2)
	if len(first) != 2 || first[0].ID != 1 || first[1].ID != 3 {
		t.Fatalf("first batch = %v, want the first two unclaimed entries in plan order", first)
	}

	for _, subtask := range first {
		claimed[subtask.ID] = struct{}{}
	}

	second := claimPlanned(planned, claimed, 2)
	if len(second) != 1 || second[0].ID != 4 {
		t.Fatalf("second batch = %v, want what is left", second)
	}

	claimed[4] = struct{}{}

	if third := claimPlanned(planned, claimed, 2); len(third) != 0 {
		t.Fatalf("a fully claimed plan must hand out nothing, got %v", third)
	}
}

func TestClaimPlanned_AsksForNothingWhenTheBatchSizeIsNotPositive(t *testing.T) {
	planned := []database.Subtask{{ID: 1}}

	for _, limit := range []int{0, -1} {
		if got := claimPlanned(planned, map[int64]struct{}{}, limit); len(got) != 0 {
			t.Errorf("limit %d returned %v", limit, got)
		}
	}
}
