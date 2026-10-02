package kube_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	plantedToken = "Bearer abcdef0123456789SECRET"
	plantedURL   = "postgres://admin:hunter2pw@db:5432/app"
	plantedKey   = "AKIAIOSFODNN7EXAMPLE"
	plantedPass  = "password=s3cr3tvalue"
)

var plantedSecrets = []string{
	"abcdef0123456789SECRET", "hunter2pw", plantedKey, "s3cr3tvalue",
}

func plantedText() string {
	return "failed " + plantedToken + " " + plantedURL + " " +
		plantedKey + " " + plantedPass
}

func assertNoSecrets(t *testing.T, texts ...string) {
	t.Helper()
	for _, text := range texts {
		for _, secret := range plantedSecrets {
			assert.NotContains(t, text, secret)
		}
	}
}

func describedTexts(d kube.Description) []string {
	var out []string
	for _, v := range d.Attributes {
		out = append(out, v.AsText())
	}
	for _, child := range d.Children {
		out = append(out, describedTexts(child)...)
	}
	return out
}

func TestEvidenceRedactionEventNote(t *testing.T) {
	ev := warningEvent("Pod", "")
	ev.Message = plantedText()
	observation, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Contains(t, observation.Note.Message, "[redacted]")
	assertNoSecrets(t, observation.Note.Message)
}

func TestEvidenceRedactionPodContainerAndConditionMessages(t *testing.T) {
	p := pod("p")
	p.Status.Message = plantedText()
	p.Status.Conditions = append(p.Status.Conditions, corev1.PodCondition{
		Type: "Custom", Status: corev1.ConditionFalse,
		Message: plantedText(),
	})
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app",
		State: corev1.ContainerState{Terminated: &corev1.
			ContainerStateTerminated{
			Reason: "Error", ExitCode: 1, Message: plantedText(),
		}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason: "Error", ExitCode: 1, Message: plantedText(),
			}},
	}, {
		Name: "web",
		State: corev1.ContainerState{Waiting: &corev1.
			ContainerStateWaiting{
			Reason: "CrashLoopBackOff", Message: plantedText(),
		}},
	}}
	d, ok := kube.PodSchema{}.Describe(p)
	assert.True(t, ok)
	texts := describedTexts(d)
	assert.NotEmpty(t, texts)
	assertNoSecrets(t, texts...)
}

func TestEvidenceRedactionNodeConditionMessage(t *testing.T) {
	n := node("n")
	n.Status.Conditions = append(n.Status.Conditions, corev1.NodeCondition{
		Type: "Custom", Status: corev1.ConditionFalse,
		Message: plantedText(),
	})
	d, ok := kube.NodeSchema{}.Describe(n)
	assert.True(t, ok)
	assertNoSecrets(t, describedTexts(d)...)
}

func TestEvidenceRedactionUnstructuredMessages(t *testing.T) {
	route := object("Widget", nil)
	route.Object["status"] = map[string]any{
		"error": map[string]any{"message": plantedText()},
		"conditions": []any{map[string]any{
			"type": "Ready", "status": "False", "message": plantedText(),
		}},
		"parents": []any{map[string]any{
			"parentRef": map[string]any{"name": "gw"},
			"conditions": []any{map[string]any{
				"type": "Accepted", "status": "False",
				"message": plantedText(),
			}},
		}},
		"listeners": []any{map[string]any{
			"name": "https",
			"conditions": []any{map[string]any{
				"type": "Programmed", "status": "False",
				"message": plantedText(),
			}},
		}},
	}
	d, ok := kube.NewUnstructuredSchema("example.com", "Widget").Describe(
		route)
	assert.True(t, ok)
	texts := describedTexts(d)
	assertNoSecrets(t, texts...)
	assert.Contains(t, texts, "parent gw: failed Bearer [redacted] "+
		"postgres://[redacted]@db:5432/app [redacted] password=[redacted]")
}

func TestEvidenceRedactionPersistentVolumeMessage(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv"},
		Status:     corev1.PersistentVolumeStatus{Message: plantedText()},
	}
	d, ok := kube.PVSchema{}.Describe(pv)
	assert.True(t, ok)
	assertNoSecrets(t, describedTexts(d)...)
}

func TestEvidenceRedactionProbeErrors(t *testing.T) {
	observations := runProbe(t, kube.ProbeConfig{
		Client: restClient(t, probeHandler("[+]ping ok\n", fixedTime())),
		Resolver: fakeResolver{
			err: errors.New(plantedText()),
		},
		Now: fixedTime,
	})
	dns := observations[kube.ClusterDNS]
	assert.Contains(t, dns.Attributes[kube.AttrProbeError].AsText(),
		"[redacted]")
	assertNoSecrets(t, dns.Attributes[kube.AttrProbeError].AsText())
}

func TestEvidenceRedactionActiveProbeErrors(t *testing.T) {
	observations := runActive(t, kube.ActiveProbeConfig{
		Targets:  []kube.ProbeTarget{{Name: "dns", Host: "example.test"}},
		Resolver: fakeResolver{err: errors.New(plantedText())},
		Interval: 60e9, FailureThreshold: 1,
	})
	got := observations[endpoint("dns")]
	assertNoSecrets(t, got.Attributes[kube.AttrProbeError].AsText())
	assert.Contains(t, got.Attributes[kube.AttrProbeError].AsText(),
		"[redacted]")
}
