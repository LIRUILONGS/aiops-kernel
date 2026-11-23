package graph

// A flow can log one decision per tool call, most of them allow, and the panel
// reads the tail of that log rather than all of it.
const (
	capabilityDecisionsDefaultLimit = 200
	capabilityDecisionsMaxLimit     = 1000
)
