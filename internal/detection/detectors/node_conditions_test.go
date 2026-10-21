package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func conditionedNode(conditions map[string]string) (
	*inventory.Model, inventory.EntityID,
) {
	m := inventory.NewModel(inventory.Options{})
	node := inventory.EntityID{Kind: kube.KindNode, Name: "worker-4"}
	var attrs map[string]inventory.Value
	for conditionType, status := range conditions {
		attrs = conditionAttrs(attrs, conditionType, status, "Reported",
			t0)
	}
	put(m, node, t0, attrs)
	return m, node
}

// Any other node condition that is True is reported once it has held for
// the sustain window, with its type in the mode and the evidence.
func TestNodeConditionTrueIsReportedAfterTheSustainWindow(t *testing.T) {
	m, node := conditionedNode(map[string]string{
		"Ready": "True", "KernelDeadlock": "True"})
	r := detection.NewRegistry(nil, NewNode(0))

	early := r.Evaluate(m, t0.Add(30*time.Second), node)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultNodePressure-30*time.Second, early.RecheckAfter)

	late := r.Evaluate(m, t0.Add(DefaultNodePressure), node)
	require.Len(t, late.Findings, 1)
	f := late.Findings[0]
	assert.Equal(t, reasons.NodeConditionPrefix+"KernelDeadlock", f.Reason)
	assert.Equal(t, detection.Mode("Node.Condition.KernelDeadlock"), f.Mode)
	assert.True(t, f.Mode.Within(detection.ModeNodeCondition))
	assert.Equal(t, detection.Warning, f.Severity)
	assert.Equal(t, "KernelDeadlock=True", f.Evidence[0].Value)
}

// Conditions that are good when True, ones read elsewhere and False ones
// are not reported.
func TestNodeConditionIgnoresHealthyAndReadOnes(t *testing.T) {
	m, node := conditionedNode(map[string]string{
		"Ready": "True", "ContainerRuntimeReady": "True",
		"CloudAvailable": "True", "NodeHealthy": "True",
		"MemoryPressure": "False", "Corrupted": "False"})
	r := detection.NewRegistry(nil, NewNode(0))

	got := r.Evaluate(m, t0.Add(time.Hour), node)

	assert.Empty(t, got.Findings)
}

// The report is bounded and ordered, so a node with many odd conditions
// stays a short, stable message.
func TestNodeConditionReportIsBounded(t *testing.T) {
	m, node := conditionedNode(map[string]string{
		"A": "True", "B": "True", "C": "True", "D": "True", "E": "True"})
	r := detection.NewRegistry(nil, NewNode(0))

	got := r.Evaluate(m, t0.Add(time.Hour), node)

	require.Len(t, got.Findings, maxNodeConditions)
	assert.Equal(t, detection.Mode("Node.Condition.A"),
		got.Findings[0].Mode)
}

// The condition's message says what the reporter saw; it is quoted, not
// summarised, so a node problem reporter's text reaches the reader.
func TestNodeConditionQuotesTheMessage(t *testing.T) {
	m, node := conditionedNode(nil)
	setCondition(m, node, "ReadonlyFilesystem", "True", "FilesystemIsReadOnly",
		"Filesystem is read-only", t0)
	r := detection.NewRegistry(nil, NewNode(0))

	got := r.Evaluate(m, t0.Add(time.Hour), node)

	require.Len(t, got.Findings, 1)
	assert.Contains(t, got.Findings[0].Evidence, detection.Evidence{
		Label: "message", Value: "Filesystem is read-only"})
}
