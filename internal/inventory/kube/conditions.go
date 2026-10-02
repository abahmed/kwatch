package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
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
func setConditions(attrs map[string]inventory.Value, conditions []condition) {
	for _, c := range conditions {
		key := AttrConditionPrefix + c.Type
		attrs[key] = inventory.Text(c.Status)
		if c.Reason != "" {
			attrs[key+AttrConditionReason] = inventory.Text(c.Reason)
		}
		if c.Message != "" && keepsMessage(c) {
			attrs[key+AttrConditionMessage] = inventory.Text(
				evidenceText(c.Message))
		}
		if !c.Since.IsZero() {
			attrs[key+AttrConditionSince] = inventory.Time(c.Since)
		}
	}
}

// failureConditionTypes are conditions where Status=True is the problem,
// so their message is evidence even though the status is True.
var failureConditionTypes = map[string]bool{
	"ReplicaFailure":      true,
	"Failed":              true,
	"FailureTarget":       true,
	"MemoryPressure":      true,
	"DiskPressure":        true,
	"PIDPressure":         true,
	"NetworkUnavailable":  true,
	"PodResizePending":    true,
	"PodResizeInProgress": true,
}

// keepsMessage skips the message of healthy conditions ("Available=True")
// to keep the model small.
func keepsMessage(c condition) bool {
	return c.Status != "True" || failureConditionTypes[c.Type]
}

// ConditionKey returns the attribute name for a condition type.
func ConditionKey(conditionType string) string {
	return AttrConditionPrefix + conditionType
}

// evidenceText is the single ingest point for free text that can carry
// secrets: credentials are redacted before the value is bounded, so a
// secret is never cut into an unrecognisable fragment.
func evidenceText(value string) string {
	return truncate(redact.Credentials(value))
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
