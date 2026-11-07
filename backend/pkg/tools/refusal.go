package tools

import (
	"errors"
	"fmt"
)

// Refusal is a handler saying no on policy grounds, as opposed to failing.
// The distinction decides what the model reads: a wrapped failure reads like a
// broken tool, and the model responds by retrying the same call or inventing a
// reason it was refused; a refusal arrives as the answer it is.
type Refusal struct {
	Reason string
}

func (r *Refusal) Error() string { return r.Reason }

// Refuse builds a refusal whose reason names the policy and what to do instead.
func Refuse(format string, args ...any) error {
	return &Refusal{Reason: fmt.Sprintf(format, args...)}
}

// refusalText reports the text to answer with when a handler refused, and
// whether it refused at all. An error that wraps a refusal still counts: the
// policy is the reason for the answer, not the wrapper.
func refusalText(err error) (string, bool) {
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return refusal.Reason, true
	}

	return "", false
}
