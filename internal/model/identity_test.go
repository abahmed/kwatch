package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectRefRoundTripAndDescription(t *testing.T) {
	ref := ObjectRef{Kind: "deployment", Namespace: "apps", Name: "api"}
	if ref.Key() != "deployment/apps/api" {
		t.Fatalf("key = %q", ref.Key())
	}
	if ref.Describe() != "deployment apps/api" {
		t.Fatalf("description = %q", ref.Describe())
	}
	parsed, ok := ParseObjectKey(ref.Key())
	if !ok || parsed != ref {
		t.Fatalf("parsed reference = %+v, %v", parsed, ok)
	}
	cluster, ok := ParseObjectKey("node//worker-a")
	if !ok || cluster.Namespace != "" || cluster.Name != "worker-a" {
		t.Fatalf("cluster reference = %+v, %v", cluster, ok)
	}
	if _, ok := ParseObjectKey("malformed"); ok {
		t.Fatal("malformed object key was accepted")
	}
}

func TestObservationHelpersPreserveEvidence(t *testing.T) {
	obs := &Observation{
		Subject: ObjectRef{Kind: "pod", Namespace: "apps", Name: "api"},
		Owner: ObjectRef{
			Kind:      "Deployment",
			Namespace: "apps",
			Name:      "backend",
		},
		RestartCount: 2,
	}
	if obs.OwnerPath() != "backend" || obs.OwnerKind() != "Deployment" {
		t.Fatalf("owner helpers = %q, %q", obs.OwnerPath(), obs.OwnerKind())
	}
	state := obs.State()
	if state == nil || state.RestartCount != 2 {
		t.Fatalf("state = %+v", state)
	}
	facts := Facts{
		ProbeEndpoint:    "http://api",
		ResourceRequests: []string{"cpu=1"},
	}
	labels := map[string]string{"app": "api"}
	obs.WithHint("failed").WithSeverity(SeverityCritical).
		WithFacts(facts).WithLabels(labels).
		WithMessage("message").WithEvidence("logs", "events", nil)
	if obs.Hint != "failed" || obs.Severity != SeverityCritical ||
		obs.Facts.ProbeEndpoint != "http://api" || obs.Message != "message" ||
		obs.Logs != "logs" || obs.Events != "events" {
		t.Fatalf("observation helpers did not preserve fields: %+v", obs)
	}
}

func TestFactsAndIncidentCloneDetachMutableFields(t *testing.T) {
	facts := Facts{ResourceRequests: []string{"cpu=1"}}
	if facts.IsZero() {
		t.Fatal("non-empty facts reported zero")
	}
	cloneFacts := facts.clone()
	cloneFacts.ResourceRequests[0] = "memory=1Gi"
	if facts.ResourceRequests[0] == cloneFacts.ResourceRequests[0] {
		t.Fatal("facts clone shared request slice")
	}
	inc := &Incident{
		Subject: Subject{Resource: "pod", Namespace: "apps", Name: "api"},
		Status: Status{
			Resources:  map[string]bool{"api-1": true},
			Containers: map[string]bool{"app": true},
		},
		Evidence: Evidence{Facts: facts},
	}
	inc.SetObject()
	copy := inc.Clone()
	copy.Resources["api-2"] = true
	copy.Facts.ResourceRequests[0] = "memory=1Gi"
	if inc.Resources["api-2"] ||
		inc.Facts.ResourceRequests[0] == "memory=1Gi" {
		t.Fatal("incident clone shared mutable fields")
	}
	require.Equal(t, ObjectRef{
		Kind: "pod", Namespace: "apps", Name: "api",
	}, inc.Ref())
}

func TestSeverityRankAndConversion(t *testing.T) {
	if SeverityCritical.Rank() <= SeverityHigh.Rank() ||
		SeverityWarning.Rank() != SeverityMedium.Rank() ||
		Severity("unknown").Rank() != 0 {
		t.Fatal("severity ranks are inconsistent")
	}
	if SeverityFromString("high") != SeverityHigh {
		t.Fatal("severity conversion changed value")
	}
}

func TestIncidentObjectRefsSortPodResources(t *testing.T) {
	inc := &Incident{
		Subject: Subject{
			Resource: "pod", Namespace: "apps", Name: "deployment",
		},
		Status: Status{Resources: map[string]bool{
			"z": true, "a": true,
		}},
	}
	refs := inc.ObjectRefs()
	if len(refs) != 2 || refs[0].Name != "a" || refs[1].Name != "z" {
		t.Fatalf("object refs = %+v", refs)
	}
}
