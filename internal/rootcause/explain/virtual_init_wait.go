package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// initWaitRows say that a Service with no ready endpoints explains the
// init container that is configured to wait for it and has run far
// longer than usual.
var initWaitRows = []Row{
	{
		Name: "init-waits-on-service",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{detection.ModeNoEndpoints}},
		Link: LinkCalls,
		Effect: Side{Kind: kube.KindContainer,
			Modes: []detection.Mode{detection.ModeInitWaiting}},
		Prior: 0.65,
	},
}

// initWaitHops lead from an init container to the Services its own
// command, arguments and environment name. Configuration alone proves
// nothing for a running application, but a container that has run far
// longer than usual and names the Service is waiting for it.
func (v *view) initWaitHops(container inventory.EntityID) []hop {
	e, ok := v.s.Model.Entity(container)
	if !ok || !attrBool(e, kube.AttrInit) {
		return nil
	}
	var out []hop
	for _, ref := range kube.ServiceCalls(e) {
		if v.s.Model.Exists(ref.Service) {
			out = append(out, hop{link: LinkCalls, to: ref.Service})
		}
	}
	return out
}
