package providers

import (
	"opsagi/pkg/database"
	"opsagi/pkg/providers/pconfig"
)

var agentByChain = map[database.MsgchainType]pconfig.ProviderOptionsType{
	database.MsgchainTypeAssistant:      pconfig.OptionsTypeAssistant,
	database.MsgchainTypeCommander:      pconfig.OptionsTypePrimaryAgent,
	database.MsgchainTypeGenerator:      pconfig.OptionsTypeGenerator,
	database.MsgchainTypeRefiner:        pconfig.OptionsTypeRefiner,
	database.MsgchainTypeReflector:      pconfig.OptionsTypeReflector,
	database.MsgchainTypeReporter:       pconfig.OptionsTypeSimple,
	database.MsgchainTypeSummarizer:     pconfig.OptionsTypeSimple,
	database.MsgchainTypeToolCallFixer:  pconfig.OptionsTypeSimpleJSON,
	database.MsgchainTypeTriage:         pconfig.OptionsTypeSimple,
	database.MsgchainTypeLogAnalyst:     pconfig.OptionsTypeSearcher,
	database.MsgchainTypeMetricsAnalyst: pconfig.OptionsTypeSearcher,
	database.MsgchainTypeChangeAnalyst:  pconfig.OptionsTypeSearcher,
}
