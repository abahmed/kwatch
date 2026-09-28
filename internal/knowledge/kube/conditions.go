package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// maxMessageLength bounds condition and status messages kept in the model.
const maxMessageLength = 256

// condition is the common shape of Kubernetes status conditions.
type condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
	Since   time.Time
}

// setConditions stores each condition as "condition.<Type>" = status with
// its reason and transition time, so detectors read every kind's
// conditions the same way.
func setConditions(attrs map[string]knowledge.Value, conditions []condition) {
	for _, c := range conditions {
		key := AttrConditionPrefix + c.Type
		attrs[key] = knowledge.Text(c.Status)
		if c.Reason != "" {
			attrs[key+AttrConditionReason] = knowledge.Text(c.Reason)
		}
		if c.Message != "" && c.Status != "True" {
			attrs[key+AttrConditionMessage] = knowledge.Text(
				truncate(c.Message))
		}
		if !c.Since.IsZero() {
			attrs[key+AttrConditionSince] = knowledge.Time(c.Since)
		}
	}
}

// ConditionKey returns the attribute name for a condition type.
func ConditionKey(conditionType string) string {
	return AttrConditionPrefix + conditionType
}

func truncate(value string) string {
	if len(value) <= maxMessageLength {
		return value
	}
	cut := maxMessageLength
	for cut > 0 && value[cut]&0xC0 == 0x80 {
		cut--
	}
	return value[:cut] + "…"
}
