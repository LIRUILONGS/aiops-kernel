package tester

import (
	"opsagi/pkg/providers/tester/cases"
)

type AgentTestResults []cases.TestResult

type ProviderTestResults struct {
	Simple     AgentTestResults `json:"simple"`
	SimpleJSON AgentTestResults `json:"simpleJson"`
	Commander  AgentTestResults `json:"commander"`
	Assistant  AgentTestResults `json:"assistant"`
	Generator  AgentTestResults `json:"generator"`
	Refiner    AgentTestResults `json:"refiner"`
	Reflector  AgentTestResults `json:"reflector"`
	Analyst    AgentTestResults `json:"analyst"`
}
