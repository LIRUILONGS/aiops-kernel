package graph

import (
	"fmt"

	"opsagi/pkg/graph/model"
)

func periodDays(period model.UsageStatsPeriod) (int32, error) {
	switch period {
	case model.UsageStatsPeriodWeek:
		return 7, nil
	case model.UsageStatsPeriodMonth:
		return 30, nil
	case model.UsageStatsPeriodQuarter:
		return 90, nil
	default:
		return 0, fmt.Errorf("invalid period: %q", period)
	}
}
