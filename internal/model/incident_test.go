package model

import (
	"encoding/json"
	"reflect"
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
			t.Errorf(
				"IncidentAction(%d).String() = %q, want %q",
				tt.action,
				got,
				tt.want,
			)
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

	want := &Incident{
		Subject: Subject{
			Key:       pi.Key,
			Reason:    pi.Reason,
			Namespace: pi.Namespace,
			Name:      pi.Name,
			Resource:  pi.Resource,
			OwnerKind: pi.OwnerKind,
			ID:        "",
		},
		Status: Status{
			Count:         pi.Count,
			FirstSeen:     pi.FirstSeen,
			LastSeen:      pi.LastSeen,
			Resources:     map[string]bool{"pod-1": true},
			PeakResources: pi.PeakResources,
			RestartCount:  pi.RestartCount,
			Severity:      pi.Severity,
			State:         pi.State,
			// ToIncident initializes an empty container set, stamps LastUpdate
			// from LastSeen, and leaves ID empty until the engine assigns one.
			Containers: map[string]bool{},
			LastUpdate: pi.LastSeen,
		},
		Evidence: Evidence{
			Hint: pi.Hint,
		},
		Delivery: Delivery{
			NotifiedSig:    pi.NotifiedSig,
			LastNotifiedAt: pi.LastNotifiedAt,
			RenotifyCount:  pi.RenotifyCount,
		},
	}

	if !reflect.DeepEqual(inc, want) {
		t.Errorf("ToIncident mismatch:\n got %+v\nwant %+v", inc, want)
	}
}

func TestPersistedIncidentToIncidentEmptyFields(t *testing.T) {
	pi := &PersistedIncident{
		Key:       "ns:dep:OOMKilled:",
		Namespace: "ns",
		Name:      "dep",
	}

	inc := pi.ToIncident()

	if inc.Resources == nil {
		t.Errorf(
			"Resources must be an empty map, never nil: the engine writes " +
				"into it",
		)
	}
	if inc.Containers == nil {
		t.Errorf("Containers should not be nil after conversion")
	}
}

func TestPersistedIncidentJSONRoundTripPreservesResolveAt(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	inc := &Incident{
		Subject: Subject{
			Key:       "ns:dep:OOMKilled:",
			Reason:    "OOMKilled",
			Namespace: "ns",
			Name:      "dep",
			Resource:  "pod",
			OwnerKind: "Deployment",
		},
		Status: Status{
			Count:      5,
			FirstSeen:  now.Add(-time.Hour),
			LastSeen:   now,
			Resources:  map[string]bool{"pod-1": true},
			Severity:   "high",
			State:      StatePendingResolve,
			ResolveAt:  now.Add(2 * time.Minute),
			Containers: map[string]bool{"app": true},
			LastUpdate: now,
		},
		Delivery: Delivery{
			NotifiedSig: "firing|high",
		},
	}

	data, err := json.Marshal(inc.ToPersisted())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var pi PersistedIncident
	if err := json.Unmarshal(data, &pi); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := pi.ToIncident()

	if got.Key != inc.Key || got.Reason != inc.Reason {
		t.Errorf("round trip lost identity: %+v", got)
	}
	if got.State != StatePendingResolve {
		t.Errorf(
			"State = %d, want %d (pending-resolve must survive restart)",
			got.State,
			StatePendingResolve,
		)
	}
	if !got.ResolveAt.Equal(now.Add(2 * time.Minute)) {
		t.Errorf(
			"ResolveAt = %v, want %v (must survive restart to finalize)",
			got.ResolveAt,
			now.Add(2*time.Minute),
		)
	}
	if !got.LastSeen.Equal(now) {
		t.Errorf("LastSeen = %v, want %v", got.LastSeen, now)
	}
}
