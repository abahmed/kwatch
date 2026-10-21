package kube

import (
	"strings"
	"sync"
	"sync/atomic"
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
		if c.Message != "" && ConditionFailing(c.Type, c.Status) {
			attrs[key+AttrConditionMessage] = inventory.Text(
				evidenceText(c.Message))
		}
		if !c.Since.IsZero() {
			attrs[key+AttrConditionSince] = inventory.Time(c.Since)
		}
	}
}

// ConditionKey returns the attribute name for a condition type.
// The type is bounded as setConditions bounds it, so a long type finds
// the attribute that was stored under its shortened name.
func ConditionKey(conditionType string) string {
	if key, ok := conditionKeys.Load(conditionType); ok {
		return key.(string)
	}
	key := AttrConditionPrefix + boundedLabel(
		conditionType, maxConditionTypeLength)
	if conditionKeyCount.Add(1) <= maxConditionKeys {
		conditionKeys.Store(conditionType, key)
	}
	return key
}

// Detectors look conditions up by a few fixed types on every evaluation,
// and redacting a type is a set of regular expressions. The keys are
// remembered, up to a cap so odd custom types cannot grow it forever.
const maxConditionKeys = 1024

var (
	conditionKeys     sync.Map
	conditionKeyCount atomic.Int64
)

// evidenceText is the single ingest point for free text that can carry
// secrets: credentials are redacted before the value is bounded, so a
// secret is never cut into an unrecognisable fragment.
func evidenceText(value string) string {
	return truncate(redact.Credentials(value))
}

// eventText is evidenceText for the message of an Event. An error ends
// with its reason, so when the message is too long its middle is cut and
// its beginning and its end are kept.
func eventText(value string) string {
	return clipMiddle(redact.Credentials(value), maxMessageLength)
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

// clipMiddle cuts value to about limit bytes by dropping its middle,
// which it marks with "…". Half of the room goes to the beginning, which
// names the object an error is about, and half to the end, which gives
// its reason, on rune boundaries.
func clipMiddle(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	head := limit / 2
	for head > 0 && value[head]&0xC0 == 0x80 {
		head--
	}
	tail := len(value) - (limit - limit/2)
	for tail < len(value) && value[tail]&0xC0 == 0x80 {
		tail++
	}
	return value[:head] + "…" + value[tail:]
}
