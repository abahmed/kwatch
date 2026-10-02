package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestPredicateReadsAsPlainEnglish(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "tls")
	cases := []struct {
		id      inventory.EntityID
		summary string
		want    string
	}{
		{secret, "TLS certificate expired 1h0m0s ago",
			"has a TLS certificate that expired one hour ago"},
		{kube.ClusterDNS, "Cluster DNS has been failing for 2m0s",
			"has been failing for two minutes"},
		{kube.Etcd, "etcd has been failing for 2m0s",
			"has been failing for two minutes"},
		{kube.Scheduler, "The scheduler has been failing for 2m0s",
			"has been failing for two minutes"},
		{inventory.CoreID(kube.KindContainer, "shop", "a-1/app"),
			"Container keeps crashing and is restarting with back-off",
			"keeps crashing"},
		{inventory.CoreID("kafkacluster", "data", "events"),
			"Reports Ready=False (InvalidBrokerConfig)",
			"is not ready (invalid broker config)"},
		{inventory.CoreID(kube.KindValidatingHook, "", "policy"),
			"Admission webhook backend policy-webhook has no ready pods",
			"has no ready pods behind service policy-webhook"},
		{secret, "DNS lookups fail", "is failing: DNS lookups fail"},
	}
	for _, c := range cases {
		if got := predicate(c.id, c.summary); got != c.want {
			t.Errorf("predicate(%q) = %q, want %q", c.summary, got, c.want)
		}
	}
}

func TestLowerFirstKeepsAcronyms(t *testing.T) {
	cases := map[string]string{
		"TLS certificate": "TLS certificate", "DNS": "DNS",
		"API server": "API server", "PVC": "PVC", "Node": "node",
		"A pod": "a pod",
	}
	for in, want := range cases {
		if got := lowerFirst(in); got != want {
			t.Errorf("lowerFirst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCamelKeepsAcronyms(t *testing.T) {
	cases := map[string]string{
		"InvalidBrokerConfig": "invalid broker config",
		"TLSError":            "TLS error",
		"MissingEndpoints":    "missing endpoints",
	}
	for in, want := range cases {
		if got := splitCamel(in); got != want {
			t.Errorf("splitCamel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNamesCapitaliseKindsButNotResourceNames(t *testing.T) {
	cases := []struct {
		id   inventory.EntityID
		want string
	}{
		{inventory.CoreID(kube.KindDeployment, "shop", "payments"),
			"payments is down."},
		{kube.Etcd, "etcd is down."},
		{kube.ClusterDNS, "Cluster DNS is down."},
		{inventory.CoreID(kube.KindHPA, "shop", "web"),
			"Autoscaler web is down."},
		{inventory.CoreID("httproute", "shop", "web"),
			"HTTP route web is down."},
	}
	for _, c := range cases {
		got := capitalName(c.id, shortName(c.id)+" is down.")
		if got != c.want {
			t.Errorf("capitalName = %q, want %q", got, c.want)
		}
	}
}

func TestOwnerInFindsTheWorkloadOfAPod(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	apiGateway := inventory.CoreID(kube.KindDeployment, "shop",
		"api-gateway")
	impact := []inventory.EntityID{api, apiGateway}
	ct := inventory.CoreID(kube.KindContainer, "shop", "api-gateway-7d-a/app")
	if got, ok := ownerIn(impact, ct); !ok || got != apiGateway {
		t.Fatalf("ownerIn = %v, %v; want api-gateway", got, ok)
	}
	other := inventory.CoreID(kube.KindPod, "billing", "api-7d-a")
	if _, ok := ownerIn(impact, other); ok {
		t.Fatal("a pod in another namespace has no owner here")
	}
}
