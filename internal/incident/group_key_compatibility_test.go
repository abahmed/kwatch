package incident

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/model"
)

func TestIncidentKeyCompatibilityGoldenForms(t *testing.T) {
	cases := []struct {
		name  string
		key   model.IncidentKey
		parts KeyParts
	}{
		{
			name: "pod owner",
			key:  BuildKey("ns", "api", "CrashLoopBackOff", ""),
			parts: KeyParts{
				Namespace: "ns", Owner: "api", Reason: "CrashLoopBackOff",
			},
		},
		{
			name: "workload owner path",
			key:  BuildKey("ns", "ns/api", "RolloutStuck", ""),
			parts: KeyParts{
				Namespace: "ns", Owner: "ns/api", Reason: "RolloutStuck",
			},
		},
		{
			name:  "cluster scoped",
			key:   BuildKey("", "coredns", "Unavailable", ""),
			parts: KeyParts{Owner: "coredns", Reason: "Unavailable"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.parts, ParseKey(tc.key))
		})
	}
}

func TestOwnerPathCompatibility(t *testing.T) {
	require.Equal(t, "ns/api", OwnerPath("ns", "api"))
	require.Equal(t, "api", OwnerPath("", "api"))
}
