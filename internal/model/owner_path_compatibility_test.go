package model

import "testing"

func TestObservationOwnerPathPreservesKeyEncodings(t *testing.T) {
	tests := []struct {
		name    string
		subject ObjectRef
		owner   ObjectRef
		want    string
	}{
		{
			name:    "pod owner is bare",
			subject: ObjectRef{Kind: "pod", Namespace: "ns"},
			owner:   ObjectRef{Kind: "Deployment", Namespace: "ns", Name: "api"},
			want:    "api",
		},
		{
			name:    "namespaced object includes namespace",
			subject: ObjectRef{Kind: "service", Namespace: "ns"},
			owner:   ObjectRef{Kind: "Service", Namespace: "ns", Name: "api"},
			want:    "ns/api",
		},
		{
			name:    "cluster scoped owner is bare",
			subject: ObjectRef{Kind: "node"},
			owner:   ObjectRef{Kind: "Node", Name: "worker-1"},
			want:    "worker-1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observation := &Observation{
				Subject: tc.subject,
				Owner:   tc.owner,
			}
			if got := observation.OwnerPath(); got != tc.want {
				t.Fatalf("OwnerPath() = %q, want %q", got, tc.want)
			}
		})
	}
}
