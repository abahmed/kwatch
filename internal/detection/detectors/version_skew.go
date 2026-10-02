package detectors

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxKubeletSkew is how many minor versions a kubelet may trail the API
// server. A kubelet newer than the API server is never supported.
const maxKubeletSkew = 3

// VersionSkew detects nodes whose kubelet is outside the supported version
// skew of the API server.
type VersionSkew struct{}

// Name implements detection.Detector.
func (VersionSkew) Name() string { return "version-skew" }

// Kinds implements detection.Detector.
func (VersionSkew) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNode}
}

// Detect implements detection.Detector.
func (VersionSkew) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	api, ok := ctx.Model.Entity(kube.APIServer)
	if !ok {
		return nil
	}
	serverMinor, ok := number(api, kube.AttrServerMinor)
	if !ok {
		return nil
	}
	kubelet := text(e, kube.AttrKubelet)
	kubeletMinor, ok := kube.ParseMinor(kubelet)
	if !ok {
		return nil
	}
	gap := int(serverMinor) - kubeletMinor
	if gap >= 0 && gap <= maxKubeletSkew {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.ClusterVersionSkew, Severity: detection.Warning,
		Since: valueSince(e, kube.AttrKubelet),
		Summary: fmt.Sprintf("Kubelet %s is outside the supported skew "+
			"of API server %s", kubelet, text(api, kube.AttrServerVersion)),
		Evidence: []detection.Evidence{
			{Label: "kubelet", Value: kubelet},
			{Label: "apiserver", Value: text(api, kube.AttrServerVersion)},
		},
	}}
}
