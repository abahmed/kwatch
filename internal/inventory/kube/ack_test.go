package kube_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var ackAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestAckAnnotationBecomesAnAttribute(t *testing.T) {
	pod := maintenancePod(map[string]string{
		kube.AckAnnotation: "looking into it"})

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, ackAt))

	assert.Equal(t, "looking into it", attrs[kube.AttrAck].AsText())
}

func TestNoAckAnnotationNoAttribute(t *testing.T) {
	pod := maintenancePod(map[string]string{"other": "x"})

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, ackAt))

	assert.NotContains(t, attrs, kube.AttrAck)
}

func TestAckWithAnEmptyValueIsStillAnAck(t *testing.T) {
	pod := maintenancePod(map[string]string{kube.AckAnnotation: ""})

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, ackAt))

	assert.Contains(t, attrs, kube.AttrAck)
}

func TestAckNoteIsBoundedRedactedAndOneLine(t *testing.T) {
	pod := maintenancePod(map[string]string{kube.AckAnnotation: "token=abc123\n" +
		"line two " + strings.Repeat("x", 5000)})

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, ackAt))

	note := attrs[kube.AttrAck].AsText()
	assert.NotContains(t, note, "abc123")
	assert.NotContains(t, note, "\n")
	assert.LessOrEqual(t, len(note), 300)
}

func TestAckRemovalDropsTheAttribute(t *testing.T) {
	translator := kube.NewTranslator(kube.PodSchema{})
	before := maintenancePod(map[string]string{kube.AckAnnotation: "x"})
	before.ResourceVersion = "1"
	after := maintenancePod(nil)
	after.ResourceVersion = "2"

	attrs := podAttributes(translator.Updated(before, after, ackAt))

	assert.NotContains(t, attrs, kube.AttrAck)
}

func TestOwnerComesFromTheLabelThenTheAnnotation(t *testing.T) {
	translator := kube.NewTranslator(kube.PodSchema{})
	byLabel := maintenancePod(map[string]string{
		kube.OwnerKey: "from-annotation"})
	byLabel.Labels = map[string]string{kube.OwnerKey: "payments"}
	byAnnotation := maintenancePod(map[string]string{
		kube.OwnerKey: "search"})

	assert.Equal(t, "payments", podAttributes(
		translator.Added(byLabel, true, ackAt))[kube.AttrOwner].AsText())
	assert.Equal(t, "search", podAttributes(
		translator.Added(byAnnotation, true, ackAt))[kube.AttrOwner].AsText())
}

func TestOwnerIgnoresOtherKeysAndEmptyValues(t *testing.T) {
	translator := kube.NewTranslator(kube.PodSchema{})
	other := maintenancePod(nil)
	other.Labels = map[string]string{"team": "payments"}
	blank := maintenancePod(nil)
	blank.Labels = map[string]string{kube.OwnerKey: " "}

	assert.NotContains(t, podAttributes(
		translator.Added(other, true, ackAt)), kube.AttrOwner)
	assert.NotContains(t, podAttributes(
		translator.Added(blank, true, ackAt)), kube.AttrOwner)
}
