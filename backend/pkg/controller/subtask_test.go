package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"opsagi/pkg/database"
)

func TestSubtaskStatusAfterChainError_CallsACancellationAPauseAndEverythingElseAFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want database.SubtaskStatus
	}{
		{"a cancelled run is a pause", context.Canceled, database.SubtaskStatusWaiting},
		{"a timed-out run is a pause", context.DeadlineExceeded, database.SubtaskStatusWaiting},
		{"a wrapped cancellation is a pause", fmt.Errorf("perform: %w", context.Canceled), database.SubtaskStatusWaiting},
		{"a model error is a failure", errors.New("API returned unexpected status code: 401"), database.SubtaskStatusFailed},
		{"a consistency error is a failure", errors.New("chain is inconsistent"), database.SubtaskStatusFailed},
		// The consistency repair runs against the same chain, so a cancellation
		// joined with it stays resumable.
		{"a cancellation joined with a consistency error is a pause", errors.Join(context.Canceled, errors.New("chain is inconsistent")), database.SubtaskStatusWaiting},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := subtaskStatusAfterChainError(tc.err); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
