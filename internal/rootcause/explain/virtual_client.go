package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// clientHops are the dependencies of a pod that is not a DNS server.
func (v *view) clientHops(id inventory.EntityID) []hop {
	out := []hop{{link: LinkResolvesVia, to: kube.ClusterDNS}}
	out = append(out, v.resolverWorkloadHops(id)...)
	out = append(out, v.dnsServingHops(id)...)
	out = append(out, v.policyCallHops(id)...)
	out = append(out, v.policyHops(id)...)
	out = append(out, v.schedulingHops(id)...)
	out = append(out, v.claimHops(id)...)
	out = append(out, v.holdHops(id)...)
	out = append(out, v.addressHolderHops(id)...)
	out = append(out, v.dnsConfigHops(id)...)
	out = append(out, v.accessHops(id)...)
	out = append(out, v.roleHops(id)...)
	out = append(out, v.unauthorizedHops(id)...)
	out = append(out, v.removedNodeHops(id)...)
	out = append(out, v.removedArchNodeHops(id)...)
	out = append(out, v.helperHops(id)...)
	out = append(out, v.calledHops(id)...)
	out = append(out, v.serviceCallHops(id)...)
	out = append(out, v.serviceNameHops(id)...)
	out = append(out, v.agentHops(id)...)
	out = append(out, v.preemptionHops(id)...)
	return out
}
