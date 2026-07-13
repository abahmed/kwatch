package model

import (
	"testing"
	"time"
)

func TestIncidentActionString(t *testing.T) {
	tests := []struct {
		action IncidentAction
		want   string
	}{
		{ActionCreate, "create"},
		{ActionUpdate, "update"},
		{ActionSkip, "skip"},
		{ActionResolved, "resolved"},
		{IncidentAction(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("IncidentAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestPersistedIncidentToIncident(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	pi := &PersistedIncident{
		Key:            "ns:dep:OOMKilled:",
		Reason:         "OOMKilled",
		Namespace:      "ns",
		Name:           "dep",
		Resource:       "pod",
		Count:          5,
		FirstSeen:      now.Add(-1 * time.Hour),
		LastSeen:       now,
		Resources:      map[string]bool{"pod-1": true},
		PeakResources:  1,
		OwnerKind:      "Deployment",
		RestartCount:   3,
		Hint:           "restart count crossed 3",
		Severity:       "high",
		State:          StateActive,
		NotifiedSig:    "firing|high",
		LastNotifiedAt: now,
		RenotifyCount:  0,
	}

	inc := pi.ToIncident()

	if inc.Key != pi.Key {
		t.Errorf("Key = %q, want %q", inc.Key, pi.Key)
	}
	if inc.Reason != pi.Reason {
		t.Errorf("Reason = %q, want %q", inc.Reason, pi.Reason)
	}
	if inc.Namespace != pi.Namespace {
		t.Errorf("Namespace = %q, want %q", inc.Namespace, pi.Namespace)
	}
	if inc.Resource != pi.Resource {
		t.Errorf("Resource = %q, want %q", inc.Resource, pi.Resource)
	}
	if inc.Count != pi.Count {
		t.Errorf("Count = %d, want %d", inc.Count, pi.Count)
	}
	if !inc.FirstSeen.Equal(pi.FirstSeen) {
		t.Errorf("FirstSeen = %v, want %v", inc.FirstSeen, pi.FirstSeen)
	}
	if !inc.LastSeen.Equal(pi.LastSeen) {
		t.Errorf("LastSeen = %v, want %v", inc.LastSeen, pi.LastSeen)
	}
	if len(inc.Resources) != 1 || !inc.Resources["pod-1"] {
		t.Errorf("Resources = %v, want map[pod-1:true]", inc.Resources)
	}
	if inc.PeakResources != pi.PeakResources {
		t.Errorf("PeakResources = %d, want %d", inc.PeakResources, pi.PeakResources)
	}
	if inc.OwnerKind != pi.OwnerKind {
		t.Errorf("OwnerKind = %q, want %q", inc.OwnerKind, pi.OwnerKind)
	}
	if inc.RestartCount != pi.RestartCount {
		t.Errorf("RestartCount = %d, want %d", inc.RestartCount, pi.RestartCount)
	}
	if inc.Hint != pi.Hint {
		t.Errorf("Hint = %q, want %q", inc.Hint, pi.Hint)
	}
	if inc.Severity != pi.Severity {
		t.Errorf("Severity = %q, want %q", inc.Severity, pi.Severity)
	}
	if inc.State != pi.State {
		t.Errorf("State = %d, want %d", inc.State, pi.State)
	}
	if inc.NotifiedSig != pi.NotifiedSig {
		t.Errorf("NotifiedSig = %q, want %q", inc.NotifiedSig, pi.NotifiedSig)
	}
	if !inc.LastNotifiedAt.Equal(pi.LastNotifiedAt) {
		t.Errorf("LastNotifiedAt = %v, want %v", inc.LastNotifiedAt, pi.LastNotifiedAt)
	}
	if inc.RenotifyCount != pi.RenotifyCount {
		t.Errorf("RenotifyCount = %d, want %d", inc.RenotifyCount, pi.RenotifyCount)
	}
	if inc.Containers == nil {
		t.Errorf("Containers should not be nil")
	}
	if inc.ID != "" {
		t.Errorf("ID should be empty after conversion, got %q", inc.ID)
	}
}

func TestPersistedIncidentToIncidentEmptyFields(t *testing.T) {
	pi := &PersistedIncident{
		Key:       "ns:dep:OOMKilled:",
		Namespace: "ns",
		Name:      "dep",
	}

	inc := pi.ToIncident()

	if inc.Resources != nil {
		t.Errorf("Resources should be nil when not set, got %v", inc.Resources)
	}
	if inc.Containers == nil {
		t.Errorf("Containers should not be nil after conversion")
	}
}
