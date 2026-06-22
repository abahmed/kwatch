package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHpaScalingErrorCreateAndResolve(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:    autoscalingv2.ScalingActive,
					Status:  corev1.ConditionFalse,
					Reason:  "FailedGetScale",
					Message: "deployment not found",
				},
			},
		},
	}

	err := h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "HPAScalingError" {
			found = true
			assert.Equal(t, model.StateActive, v.State)
		}
	}
	assert.True(t, found, "HPAScalingError incident should exist")

	// Clear the condition → should resolve
	hpa.Status.Conditions[0].Status = corev1.ConditionTrue
	hpa.Status.Conditions[0].Reason = "Ready"
	err = h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "HPAScalingError should resolve when condition clears")
}

func TestHpaMaxedOutStillWorksIndependently(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})

	now := time.Now()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		HpaMonitor: config.HpaMonitor{SustainedMinutes: 0},
	}, e, testAlertMgr)
	h.(*handler).now = func() time.Time { return now }

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "maxed-hpa",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 5,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 5,
			CurrentReplicas: 3,
		},
	}

	err := h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "HPAMaxedOut" {
			found = true
		}
	}
	assert.True(t, found, "HPAMaxedOut incident should exist")
}

func TestHpaScalingErrorAndMaxedCoexist(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})

	now := time.Now()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		HpaMonitor: config.HpaMonitor{SustainedMinutes: 0},
	}, e, testAlertMgr)
	h.(*handler).now = func() time.Time { return now }

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "both-hpa",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 5,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 5,
			CurrentReplicas: 3,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:    autoscalingv2.AbleToScale,
					Status:  corev1.ConditionFalse,
					Reason:  "FailedGetResourceMetric",
					Message: "container missing memory request",
				},
			},
		},
	}

	err := h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	snap := e.Snapshot()
	var foundScalingErr, foundMaxed bool
	for _, v := range snap {
		switch v.Reason {
		case "HPAScalingError":
			foundScalingErr = true
		case "HPAMaxedOut":
			foundMaxed = true
		}
	}
	assert.True(t, foundScalingErr, "HPAScalingError should exist")
	assert.True(t, foundMaxed, "HPAMaxedOut should exist")
}

func TestHpaScalingErrorOnlyResolvedWhenConditionClears(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	now := time.Now()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		HpaMonitor: config.HpaMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)
	h.(*handler).now = func() time.Time { return now }

	// HPA that is maxed AND has a scaling error
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "combined",
			Namespace: "default",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 5,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 5,
			CurrentReplicas: 3,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:    autoscalingv2.ScalingActive,
					Status:  corev1.ConditionFalse,
					Reason:  "FailedGetScale",
					Message: "deployment missing",
				},
			},
		},
	}

	// First pass: both incidents created
	err := h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	// Clear only the scaling error, HPA stays maxed
	hpa.Status.Conditions[0].Status = corev1.ConditionTrue
	hpa.Status.Conditions[0].Reason = "Ready"
	err = h.ProcessHorizontalPodAutoscalerObject(hpa, false)
	assert.NoError(t, err)

	// Should have resolved HPAScalingError but NOT HPAMaxedOut
	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "exactly one resolve (HPAScalingError), not the maxed incident")

	snap := e.Snapshot()
	for _, v := range snap {
		if v.Reason == "HPAScalingError" {
			assert.Equal(t, model.StateResolved, v.State, "HPAScalingError should be resolved")
		}
	}
}

func TestHpaNilAndDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)

	assert.NoError(t, h.ProcessHorizontalPodAutoscalerObject(nil, false))

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "del-hpa",
			Namespace: "default",
		},
	}
	assert.NoError(t, h.ProcessHorizontalPodAutoscalerObject(hpa, true))
}
