package kube

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxMessageLength bounds condition and status messages kept in the model.
const maxMessageLength = 256

// Bounds on the free-form parts of a condition that a custom resource
// controls. The type becomes part of an attribute name.
const (
	maxConditionTypeLength   = 64
	maxConditionReasonLength = 128
)

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
//
// A type with a dot (a pod readiness gate such as "example.com/ready")
// is left out: its attribute names "condition.<type>.reason" could not be
// told from the reason of another condition.
func setConditions(attrs map[string]inventory.Value, conditions []condition) {
	for _, c := range conditions {
		if strings.Contains(c.Type, ".") {
			continue
		}
		key := ConditionKey(c.Type)
		attrs[key] = inventory.Text(c.Status)
		if c.Reason != "" {
			attrs[key+AttrConditionReason] = inventory.Text(
				boundedLabel(c.Reason, maxConditionReasonLength))
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
	// DisruptionTarget=True names who preempted or evicted the pod.
	"DisruptionTarget": true,
	// A terminating namespace reports what keeps it alive as True.
	"NamespaceDeletionDiscoveryFailure": true,
	"NamespaceDeletionContentFailure":   true,
	"NamespaceContentRemaining":         true,
	"NamespaceFinalizersRemaining":      true,
}

// keepsMessage skips the message of healthy conditions ("Available=True")
// to keep the model small.
func keepsMessage(c condition) bool {
	return c.Status != "True" || failureConditionTypes[c.Type]
}

// ConditionKey returns the attribute name for a condition type.
// The type is bounded as setConditions bounds it, so a long type finds
// the attribute that was stored under its shortened name.
func ConditionKey(conditionType string) string {
	return AttrConditionPrefix + boundedLabel(
		conditionType, maxConditionTypeLength)
}

// evidenceText is the single ingest point for free text that can carry
// secrets: credentials are redacted before the value is bounded, so a
// secret is never cut into an unrecognisable fragment.
func evidenceText(value string) string {
	return truncate(redact.Credentials(value))
}

// boundedLabel redacts credentials, then cuts the value to limit bytes.
func boundedLabel(value string, limit int) string {
	return truncateTo(redact.Credentials(value), limit)
}

func truncate(value string) string {
	return truncateTo(value, maxMessageLength)
}

func truncateTo(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && value[cut]&0xC0 == 0x80 {
		cut--
	}
	return value[:cut] + "…"
}
