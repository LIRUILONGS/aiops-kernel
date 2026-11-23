package graph

import "time"

// A flow can log one decision per tool call, most of them allow, and the panel
// reads the tail of that log rather than all of it.
const (
	capabilityDecisionsDefaultLimit = 200
	capabilityDecisionsMaxLimit     = 1000
)

// capabilityApprovalTTL bounds an approval in time, not in uses: a grant is a
// standing answer to the same call, and an operator who wanted one call would
// have to sit and revoke it. Four hours outlives the flow it was given for.
const capabilityApprovalTTL = 4 * time.Hour

func clampCapabilityRowLimit(limit *int) int {
	requested := capabilityDecisionsDefaultLimit
	if limit != nil && *limit > 0 {
		requested = *limit
	}
	if requested > capabilityDecisionsMaxLimit {
		requested = capabilityDecisionsMaxLimit
	}

	return requested
}
