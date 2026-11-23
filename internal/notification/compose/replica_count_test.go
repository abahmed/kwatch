package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestWithReplicaCount(t *testing.T) {
	root := inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "billing", Name: "api"}
	pod := inventory.EntityID{Kind: kube.KindPod,
		Namespace: "billing", Name: "api-1"}
	state := "has not been ready for three minutes"
	cases := []struct {
		name  string
		count *incident.ReplicaCount
		lead  inventory.EntityID
		state string
		want  string
	}{
		{"none ready", &incident.ReplicaCount{Desired: 2}, pod, state,
			"has had no ready replicas (0 of 2) for three minutes"},
		{"some ready", &incident.ReplicaCount{Desired: 3, Ready: 1}, pod,
			state, "has had only 1 of 3 replicas ready for three minutes"},
		{"unknown count", nil, pod, state, state},
		{"other state", &incident.ReplicaCount{Desired: 2}, pod,
			"keeps crashing", "keeps crashing"},
		{"not a pod", &incident.ReplicaCount{Desired: 2}, root, state,
			state},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := caseFacts{p: incident.Incident{Root: root},
				lead:     detection.Finding{Entity: tc.lead},
				replicas: tc.count}
			if got := withReplicaCount(f, tc.state); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
