package graph

import (
	"sort"
	"time"

	"opsagi/pkg/capability"
	"opsagi/pkg/config"
	"opsagi/pkg/graph/model"
)

// A flow can log one decision per tool call, most of them allow, and the panel
// reads the tail of that log rather than all of it.
const (
	capabilityDecisionsDefaultLimit = 200
	capabilityDecisionsMaxLimit     = 1000
)

// A flow issues one lease per subtask, and a panel shows the last few rather
// than a history that grows with the flow.
const (
	capabilityLeasesDefaultLimit = 20
	capabilityLeasesMaxLimit     = 100
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

func clampCapabilityLeaseLimit(limit *int) int {
	requested := capabilityLeasesDefaultLimit
	if limit != nil && *limit > 0 {
		requested = *limit
	}
	if requested > capabilityLeasesMaxLimit {
		requested = capabilityLeasesMaxLimit
	}

	return requested
}

// capabilityBudgets builds the call limits a lease was issued under. They come
// from the policy in force rather than from the lease row, which is why the
// panel shows what the runtime would apply now, not what it applied at issue.
func capabilityBudgets(cfg *config.Config, mode string) []*model.CapabilityBudget {
	policy := capability.DefaultPolicy()
	if cfg != nil && cfg.CapabilityPolicy != nil {
		policy = *cfg.CapabilityPolicy
	}

	budgets := policy.RuleFor(mode).Budgets
	tools := make([]string, 0, len(budgets))
	for tool := range budgets {
		tools = append(tools, tool)
	}
	sort.Strings(tools)

	converted := make([]*model.CapabilityBudget, 0, len(tools))
	for _, tool := range tools {
		limit := budgets[tool]
		converted = append(converted, &model.CapabilityBudget{Tool: tool, Limit: limit})
	}

	return converted
}
