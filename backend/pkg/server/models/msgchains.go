package models

import (
	"fmt"

	"github.com/jinzhu/gorm"
)

type MsgchainType string

const (
	MsgchainTypeReporter      MsgchainType = "reporter"
	MsgchainTypeGenerator     MsgchainType = "generator"
	MsgchainTypeRefiner       MsgchainType = "refiner"
	MsgchainTypeReflector     MsgchainType = "reflector"
	MsgchainTypeSummarizer    MsgchainType = "summarizer"
	MsgchainTypeToolCallFixer MsgchainType = "tool_call_fixer"
	MsgchainTypeAssistant     MsgchainType = "assistant"
	MsgchainTypeCommander     MsgchainType = "commander"
	MsgchainTypeTriage        MsgchainType = "triage"
	MsgchainTypeLogAnalyst    MsgchainType = "log_analyst"
	MsgchainTypeMetricsAnalyst MsgchainType = "metrics_analyst"
	MsgchainTypeChangeAnalyst  MsgchainType = "change_analyst"
)

func (e *MsgchainType) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MsgchainType(s)
	case string:
		*e = MsgchainType(s)
	default:
		return fmt.Errorf("unsupported scan type for MsgchainType: %T", src)
	}
	return nil
}

func (s MsgchainType) String() string {
	return string(s)
}

// Valid is function to control input/output data
func (ml MsgchainType) Valid() error {
	switch ml {
	case MsgchainTypeReporter,
		MsgchainTypeGenerator, MsgchainTypeRefiner,
		MsgchainTypeReflector,
		MsgchainTypeSummarizer, MsgchainTypeToolCallFixer,
		MsgchainTypeAssistant, MsgchainTypeCommander,
		MsgchainTypeTriage, MsgchainTypeLogAnalyst,
		MsgchainTypeMetricsAnalyst, MsgchainTypeChangeAnalyst:
		return nil
	default:
		return fmt.Errorf("invalid MsgchainType: %s", ml)
	}
}

// Validate is function to use callback to control input/output data
func (ml MsgchainType) Validate(db *gorm.DB) {
	if err := ml.Valid(); err != nil {
		db.AddError(err)
	}
}
