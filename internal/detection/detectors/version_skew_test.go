package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func detectVersionSkew(
	serverMinor *float64, kubelet string,
) []detection.Finding {
	model := newTestModel()
	if serverMinor != nil {
		observeEntity(model, kube.APIServer, podNodeNow,
			map[string]inventory.Value{
				kube.AttrServerVersion: inventory.Text("v1.30.1"),
				kube.AttrServerMinor:   inventory.Number(*serverMinor),
			})
	}
	observeEntity(model, podNodeID, podNodeNow, map[string]inventory.Value{
		kube.AttrKubelet: inventory.Text(kubelet),
	})
	return classified(VersionSkew{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, podNodeID)))
}

func TestVersionSkewFlagsUnsupportedKubelets(t *testing.T) {
	server := 30.0
	cases := map[string]string{
		"too old":            "v1.26.5",
		"newer than server":  "v1.31.0",
		"distribution build": "v1.25.9-gke.400",
	}
	for name, kubelet := range cases {
		t.Run(name, func(t *testing.T) {
			found := detectVersionSkew(&server, kubelet)
			require.Len(t, found, 1)
			assert.Equal(t, reasons.ClusterVersionSkew, found[0].Reason)
			assert.Equal(t, "Cluster.VersionSkew", string(found[0].Mode))
			assert.Equal(t, detection.Degraded, found[0].Health)
		})
	}
}

func TestVersionSkewIgnoresSupportedOrUnknownVersions(t *testing.T) {
	server := 30.0
	assert.Empty(t, detectVersionSkew(&server, "v1.27.0"))
	assert.Empty(t, detectVersionSkew(&server, "v1.30.4"))
	assert.Empty(t, detectVersionSkew(&server, "garbage"))
	assert.Empty(t, detectVersionSkew(nil, "v1.20.0"))
}
