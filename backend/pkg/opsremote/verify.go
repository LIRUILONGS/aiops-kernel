package opsremote

import (
	"fmt"
	"strings"
)

// Verdict states what a change did, in words a conclusion can quote. Every one
// of them is decided by comparing two fingerprints: a model may argue about a
// diagnosis, but "did anything measurable move" is arithmetic.
const (
	// VerdictImproved means every material delta moved the right way.
	VerdictImproved = "improved"
	// VerdictWorsened means at least one material delta moved the wrong way.
	VerdictWorsened = "worsened"
	// VerdictChanged means the object changed in a way that is neither good nor
	// bad on its own: a container that was down is up. The row says which, the
	// overall verdict refuses to guess.
	VerdictChanged = "changed"
	// VerdictUnchanged means nothing about the object moved past the materiality
	// floor. A repair that changed nothing measurable did not demonstrably
	// repair anything.
	VerdictUnchanged = "unchanged"
	// VerdictUnverified means there is no comparison to make. It is the answer
	// whenever evidence is missing, and it must never be reported as a repair.
	VerdictUnverified = "unverified"
)

// Change is the result of measuring before, changing, measuring after.
type Change struct {
	State  string
	Reason string
	// Deltas are the material rows about the object the change touched. They are
	// the verdict's only input.
	Deltas []Delta
	// Bystanders are the material rows about everything else. They are shown,
	// never counted: a container restart does not improve or worsen the host's
	// load, and crediting it with either turns noise into evidence.
	Bystanders  []Delta
	BeforeLabel string
	AfterLabel  string
}

// JudgeChange compares the two fingerprints around a change, restricted to the
// field prefixes the change can move. Missing evidence is its own answer: a nil
// baseline, a failed collection, two different hosts or an object the fingerprint
// does not carry all yield unverified, because the alternative is a repair claim
// nobody can check.
func JudgeChange(before, after *Fingerprint, prefixes []string) Change {
	switch {
	case before == nil:
		return Change{State: VerdictUnverified, Reason: "变更前没有采到基线"}
	case after == nil:
		return Change{State: VerdictUnverified, Reason: "变更后没有采到基线"}
	case before.Host != "" && after.Host != "" && before.Host != after.Host:
		return Change{
			State:  VerdictUnverified,
			Reason: fmt.Sprintf("前后基线不是同一台主机（%s 与 %s）", before.Host, after.Host),
		}
	case len(prefixes) == 0:
		return Change{State: VerdictUnverified, Reason: "这个动作的效果不在主机指纹里，没有可比的对象"}
	}

	change := Change{State: VerdictUnchanged, BeforeLabel: before.Label, AfterLabel: after.Label}

	var improved, worsened int
	for _, delta := range CompareFingerprints(*before, *after) {
		if !delta.Material {
			continue
		}
		if !prefixed(delta.Field, prefixes) {
			change.Bystanders = append(change.Bystanders, delta)
			continue
		}
		change.Deltas = append(change.Deltas, delta)
		switch delta.Verdict {
		case "改善":
			improved++
		case "恶化":
			worsened++
		}
	}

	switch {
	case worsened > 0:
		change.State = VerdictWorsened
		change.Reason = fmt.Sprintf("%d 项相关指标变差，%d 项变好", worsened, improved)
	case len(change.Deltas) > 0 && improved == len(change.Deltas):
		change.State = VerdictImproved
		change.Reason = fmt.Sprintf("%d 项相关指标全部变好", improved)
	case len(change.Deltas) > 0:
		change.State = VerdictChanged
		change.Reason = fmt.Sprintf("%d 项相关指标有变化，其中 %d 项变好、%d 项变差", len(change.Deltas), improved, worsened)
	default:
		change.Reason = "这个对象在指纹里没有任何变化，这次改动没有可测量的效果"
	}

	return change
}

// Verified reports whether the change can be quoted as evidence of anything.
func (c Change) Verified() bool { return c.State != VerdictUnverified }

func prefixed(field string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(field, prefix) {
			return true
		}
	}

	return false
}
