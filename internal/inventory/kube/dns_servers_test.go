package kube

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

var dnsAt = time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)

// dnsModel builds a model from observations dated dnsAt.
func dnsModel(t *testing.T, obs ...inventory.Observation) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	for _, o := range obs {
		o.At, o.Source = dnsAt, "test"
		if _, err := m.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func dnsSeen(
	id inventory.EntityID, attrs map[string]inventory.Value,
) inventory.Observation {
	return inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs}
}

func dnsLink(id inventory.EntityID, rel inventory.RelationType,
	to ...inventory.EntityID) inventory.Observation {
	return inventory.Observation{Kind: inventory.Related, Entity: id,
		Relation: rel, Targets: to}
}

// dnsSetup is a DNS Service selecting one pod owned by workload, with
// a ReplicaSet in between for a Deployment.
func dnsSetup(
	svc, workload inventory.EntityID, selector string,
) ([]inventory.Observation, inventory.EntityID) {
	pod := inventory.CoreID(KindPod, svc.Namespace, workload.Name+"-x1")
	obs := []inventory.Observation{
		dnsSeen(svc, map[string]inventory.Value{
			AttrSelector: inventory.Text(selector)}),
		dnsSeen(workload, nil),
		dnsSeen(pod, map[string]inventory.Value{
			AttrPodIP: inventory.Text("10.244.0.9")}),
		dnsLink(svc, inventory.Selects, pod),
	}
	owner := workload
	if workload.Kind == KindDeployment {
		owner = inventory.CoreID(KindReplicaSet, svc.Namespace,
			workload.Name+"-7f")
		obs = append(obs, dnsSeen(owner, nil),
			dnsLink(owner, inventory.OwnedBy, workload))
	}
	return append(obs, dnsLink(pod, inventory.OwnedBy, owner)), pod
}

func TestDNSServersFoundThroughTheDNSService(t *testing.T) {
	tests := []struct {
		name          string
		svc, workload inventory.EntityID
		selector      string
	}{
		{"openshift daemonset",
			inventory.CoreID(KindService, "openshift-dns", "dns-default"),
			inventory.CoreID(KindDaemonSet, "openshift-dns", "dns-default"),
			"dns.operator.openshift.io/daemonset-dns=default"},
		{"rke2 chart",
			inventory.CoreID(KindService, "kube-system",
				"rke2-coredns-rke2-coredns"),
			inventory.CoreID(KindDeployment, "kube-system",
				"rke2-coredns-rke2-coredns"),
			"app.kubernetes.io/instance=rke2-coredns"},
		{"helm release prefix",
			inventory.CoreID(KindService, "kube-system", "dns-svc"),
			inventory.CoreID(KindDeployment, "kube-system", "acme-coredns"),
			dnsSelector},
		{"kube-dns service, renamed workload",
			inventory.CoreID(KindService, "kube-system", "kube-dns"),
			inventory.CoreID(KindDeployment, "kube-system", "cluster-dns"),
			"app=cluster-dns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs, pod := dnsSetup(tt.svc, tt.workload, tt.selector)
			model := dnsModel(t, obs...)
			if got := DNSServerPods(model); len(got) != 1 || got[0] != pod {
				t.Fatalf("pods = %v, want %v", got, pod)
			}
			if !IsDNSServer(model, pod) ||
				!IsDNSWorkload(model, tt.workload) {
				t.Fatal("the DNS pod or its workload was not recognised")
			}
		})
	}
}

func TestDNSServersFallBackToTheWorkloadName(t *testing.T) {
	workload := inventory.CoreID(KindDeployment, "kube-system", "coredns")
	rs := inventory.CoreID(KindReplicaSet, "kube-system", "coredns-5d")
	pod := inventory.CoreID(KindPod, "kube-system", "coredns-5d-a")
	other := inventory.CoreID(KindPod, "kube-system", "metrics-a")
	model := dnsModel(t, dnsSeen(workload, nil), dnsSeen(rs, nil),
		dnsSeen(pod, nil), dnsSeen(other, nil),
		dnsLink(rs, inventory.OwnedBy, workload),
		dnsLink(pod, inventory.OwnedBy, rs))
	if got := DNSServerPods(model); len(got) != 1 || got[0] != pod {
		t.Fatalf("pods = %v", got)
	}
	if IsDNSWorkload(model, inventory.CoreID(KindDeployment,
		"kube-system", "metrics")) {
		t.Fatal("an unrelated workload counted as DNS")
	}
}

func TestDNSServersIgnoreUnrelatedServices(t *testing.T) {
	obs, pod := dnsSetup(inventory.CoreID(KindService, "kube-system",
		"metrics-server"), inventory.CoreID(KindDeployment, "kube-system",
		"metrics-server"), "k8s-app=metrics-server")
	if IsDNSServer(dnsModel(t, obs...), pod) {
		t.Fatal("a pod behind an unrelated Service counted as DNS")
	}
}
