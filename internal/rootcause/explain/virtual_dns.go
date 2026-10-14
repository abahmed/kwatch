package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// dnsWorkloadHops lead from the coredns or kube-dns Deployment to the
// cluster DNS.
// Its pods run but never pass readiness when the cluster DNS is down,
// so the Deployment's own never-ready finding belongs to the DNS outage
// too, not to a second incident.
func (v *view) dnsWorkloadHops(id inventory.EntityID) []hop {
	if id.Kind == kube.KindDeployment && id.Namespace == "kube-system" &&
		kube.DNSServerNames[id.Name] {
		return []hop{{link: LinkContains, to: kube.ClusterDNS}}
	}
	return nil
}

// dnsServer reports whether a pod serves the cluster DNS: it belongs
// to the coredns or kube-dns Deployment of kube-system.
func (v *view) dnsServer(pod inventory.EntityID) bool {
	if pod.Namespace != "kube-system" {
		return false
	}
	top := rootcause.TopOwner(v.s.Model, pod)
	return top != pod && kube.DNSServerNames[top.Name]
}
