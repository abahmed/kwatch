package status

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// controlPlane lists the components kwatch probes, in reading order.
var controlPlane = []struct {
	id   inventory.EntityID
	name string
}{
	{kube.APIServer, "API server"},
	{kube.Etcd, "etcd"},
	{kube.Scheduler, "scheduler"},
	{kube.ControllerManager, "controller manager"},
	{kube.ClusterDNS, "cluster DNS"},
}

// ControlPlane is the state of each probed component: "problem" with
// the finding's summary, "ok" when its probe reports healthy, and
// "unknown" when kwatch has not probed it. A component kwatch does not
// know at all is left out.
func ControlPlane(
	model inventory.Reader, findings []detection.Finding,
) []Component {
	var out []Component
	for _, c := range controlPlane {
		e, ok := model.Entity(c.id)
		if !ok {
			continue
		}
		out = append(out, component(c.name, e, findings))
	}
	return out
}

func component(
	name string, e inventory.Entity, findings []detection.Finding,
) Component {
	for _, f := range findings {
		if f.Entity == e.ID && !f.Advisory {
			return Component{Name: name, State: "problem",
				Detail: redact.Evidence(f.Summary)}
		}
	}
	healthy, known := e.Attribute(kube.AttrHealthy)
	if ok, _ := healthy.Value.AsBool(); known && ok {
		return Component{Name: name, State: "ok"}
	} else if known {
		return Component{Name: name, State: "problem",
			Detail: "its probe is failing"}
	}
	return Component{Name: name, State: "unknown"}
}
