package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/status"
)

func TestKwatchStatusSaysWhereTheClusterNameCameFrom(t *testing.T) {
	cases := []struct {
		name, cluster, source string
	}{
		{"configured", "prod-eu-1", "configured"},
		{"detected", "prod-eu-1", "from the node label x/cluster-name"},
		{"source unknown", "prod-eu-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			s.SetStatus(func() status.Report {
				return status.Report{Cluster: tc.cluster}
			})
			s.SetClusterNameSource(tc.source)

			got, _ := readKwatch(t, s)

			require.NotNil(t, got.ClusterName)
			assert.Equal(t, tc.cluster, got.ClusterName.Name)
			assert.Equal(t, tc.source, got.ClusterName.Source)
			assert.Empty(t, got.ClusterName.Notice)
		})
	}
}

func TestKwatchStatusWithoutAClusterNameSaysHowToSetOne(t *testing.T) {
	s := newTestServer()

	got, _ := readKwatch(t, s)

	require.NotNil(t, got.ClusterName)
	assert.Empty(t, got.ClusterName.Name)
	assert.Equal(t, "This cluster has no name. Set app.clusterName so "+
		"alerts and the dashboard say which cluster they are about.",
		got.ClusterName.Notice)
}
