package graph

import "opsagi/pkg/graph/model"

// An absent type keeps the behaviour every existing flow has: the planner decides
// per plan entry whether a subtask is an incident or a performance task.
const flowTypeAuto = "auto"

func convertFlowType(flowType *model.FlowType) string {
	if flowType == nil {
		return flowTypeAuto
	}

	return string(*flowType)
}
