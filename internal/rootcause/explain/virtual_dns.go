package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dnsWorkloadHops lead from the workload that serves the cluster DNS,
// of any kind (see kube.DNSServerPods), to the cluster DNS.
// Its pods run but never pass readiness when the cluster DNS is down,
// so the Deployment's own never-ready finding belongs to the DNS outage
// too, not to a second incident.
func (v *view) dnsWorkloadHops(id inventory.EntityID) []hop {
	if dnsWorkloadKinds[id.Kind] && kube.IsDNSWorkload(v.s.Model, id) {
		return []hop{{link: LinkContains, to: kube.ClusterDNS}}
	}
	return nil
}

// dnsServer reports whether a pod serves the cluster DNS, found
// through the DNS Service (see kube.DNSServerPods).
func (v *view) dnsServer(pod inventory.EntityID) bool {
	return kube.IsDNSServer(v.s.Model, pod)
}

// dnsWorkloadKinds are the kinds that can run the cluster DNS.
var dnsWorkloadKinds = map[inventory.Kind]bool{
	kube.KindDeployment: true, kube.KindDaemonSet: true,
	kube.KindStatefulSet: true, kube.KindReplicaSet: true,
}
