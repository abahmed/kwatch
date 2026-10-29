package kube

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// dnsServiceIDs are the Services that front the cluster DNS by name:
// kube-dns in kube-system (kubeadm, kind, managed clusters, RKE2 keeps
// the name for kubelet) and dns-default in openshift-dns (OpenShift).
var dnsServiceIDs = []inventory.EntityID{
	inventory.CoreID(KindService, "kube-system", "kube-dns"),
	inventory.CoreID(KindService, "openshift-dns", "dns-default"),
}

// dnsSelector is the selector the CoreDNS charts and manifests give the
// DNS Service, whatever its name.
const dnsSelector = "k8s-app=kube-dns"

// dnsServerNames are the workload names that serve the cluster DNS in
// kube-system. They are only the fallback when no DNS Service is known
// or it selects no pod.
var dnsServerNames = map[string]bool{"coredns": true, "kube-dns": true}

// maxOwnerDepth bounds the owner walk: pod, ReplicaSet, Deployment.
const maxOwnerDepth = 3

// DNSServerPods returns the pods that serve the cluster DNS, sorted:
// the pods the DNS Services select, of any workload kind (an OpenShift
// DaemonSet, an RKE2 or Helm Deployment with a release prefix). Without
// them it falls back to the kube-system pods of a coredns or kube-dns
// workload.
func DNSServerPods(model inventory.Reader) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, svc := range dnsServices(model) {
		for _, pod := range model.Related(svc, inventory.Selects,
			inventory.Outgoing) {
			if pod.Kind == KindPod && !seen[pod] {
				seen[pod] = true
				out = append(out, pod)
			}
		}
	}
	if len(out) == 0 {
		out = namedDNSPods(model)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// IsDNSServer reports whether pod serves the cluster DNS.
func IsDNSServer(model inventory.Reader, pod inventory.EntityID) bool {
	for _, id := range DNSServerPods(model) {
		if id == pod {
			return true
		}
	}
	return false
}

// IsDNSWorkload reports whether id is the workload, of any kind, whose
// pods serve the cluster DNS.
func IsDNSWorkload(model inventory.Reader, id inventory.EntityID) bool {
	for _, pod := range DNSServerPods(model) {
		if top := TopOwner(model, pod); top != pod && top == id {
			return true
		}
	}
	return false
}

// dnsServices are the DNS Services the model has: the known names, and
// any kube-system Service named for CoreDNS or selecting its label.
func dnsServices(model inventory.Reader) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range dnsServiceIDs {
		if model.Exists(id) {
			out = append(out, id)
		}
	}
	for _, id := range model.EntitiesIn(KindService, "kube-system") {
		if id.Name == "kube-dns" {
			continue
		}
		e, ok := model.Entity(id)
		if !ok {
			continue
		}
		selector, _ := e.Attribute(AttrSelector)
		if strings.Contains(id.Name, "coredns") ||
			hasSelector(selector.Value.AsText(), dnsSelector) {
			out = append(out, id)
		}
	}
	return out
}

func hasSelector(selector, want string) bool {
	for _, part := range strings.Split(selector, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// namedDNSPods is the fallback: kube-system pods whose top owner is
// named coredns or kube-dns.
func namedDNSPods(model inventory.Reader) []inventory.EntityID {
	var out []inventory.EntityID
	for _, pod := range model.EntitiesIn(KindPod, "kube-system") {
		top := TopOwner(model, pod)
		if top != pod && dnsServerNames[top.Name] {
			out = append(out, pod)
		}
	}
	return out
}

// TopOwner is the pod's top owner: the Deployment behind its
// ReplicaSet, a DaemonSet or StatefulSet, or the pod itself.
func TopOwner(
	model inventory.Reader, pod inventory.EntityID,
) inventory.EntityID {
	id := pod
	for range maxOwnerDepth {
		owners := model.Related(id, inventory.OwnedBy, inventory.Outgoing)
		if len(owners) == 0 {
			break
		}
		id = owners[0]
	}
	return id
}
