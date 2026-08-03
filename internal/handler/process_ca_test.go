package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestClusterAutoscalerSustainedGateResetOnSuccess(t *testing.T) {
	now := time.Now()
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	hh := h
	hh.now = func() time.Time { return now }

	caEvent := func(reason string) *corev1.Event {
		return &corev1.Event{Reason: reason, InvolvedObject: corev1.ObjectReference{Name: "ca-1"}}
	}

	// First failure arms the sustain gate — no alert yet.
	hh.ProcessClusterAutoscalerEvent(caEvent("FailedToScaleUp"))
	assert.Equal(t, 0, e.ActiveCount(), "first failure must only arm the gate")

	// Same failure persisted past the sustain window → alert.
	now = now.Add(6 * time.Minute)
	hh.ProcessClusterAutoscalerEvent(caEvent("FailedToScaleUp"))
	assert.Equal(t, 1, e.ActiveCount(), "sustained failure must alert")

	// A successful scale event clears the gate.
	now = now.Add(1 * time.Minute)
	hh.ProcessClusterAutoscalerEvent(caEvent("TriggeredScaleUp"))
	if _, ok := hh.firstCaBlocked.get("FailedToScaleUp"); ok {
		t.Fatal("TriggeredScaleUp must reset the FailedToScaleUp gate")
	}

	// A fresh failure needs a new sustain window before alerting again.
	hh.ProcessClusterAutoscalerEvent(caEvent("FailedToScaleUp"))
	assert.Equal(t, 1, e.ActiveCount(), "gate must re-arm, no immediate re-alert")
	first, ok := hh.firstCaBlocked.get("FailedToScaleUp")
	if !ok {
		t.Fatal("fresh failure must re-arm the gate")
	}
	assert.Equal(t, now, first, "gate must start a fresh sustain window")
}
