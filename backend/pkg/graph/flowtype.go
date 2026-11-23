package graph

import "opsagi/pkg/graph/model"

// An absent type keeps the behaviour every existing flow has: the planner decides
// per plan entry whether a subtask is an incident or a performance task.
const flowTypeAuto = "auto"

// An absent switch keeps the behaviour every existing flow has: no lease is
// issued and no tool call is checked.
const capabilityModeOff = "off"

func convertFlowType(flowType *model.FlowType) string {
	if flowType == nil {
		return flowTypeAuto
	}

	return string(*flowType)
}

func convertCapabilityMode(mode *model.CapabilityMode) string {
	if mode == nil {
		return capabilityModeOff
	}

	return string(*mode)
}
