package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestPredicateReadsAsPlainEnglish(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "tls")
	node := inventory.CoreID(kube.KindNode, "", "n1")
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
		{inventory.CoreID(kube.KindContainer, "staging", "draftor-1/app"),
			"CPU is throttled 75% of the time; requests slow down",
			"is throttled on CPU 75% of the time"},
		{secret, "DNS lookups fail", "is failing: DNS lookups fail"},
		{node, "Node is short on memory and its pods' limits add up " +
			"to 155% of its memory; pods are killed",
			"is short on memory and its pods' limits add up to " +
				"155% of its memory"},
		{node, "Node is under CPU pressure: workloads stall on CPU",
			"is under CPU pressure: workloads stall on CPU"},
		{inventory.CoreID(kube.KindHPA, "istio-system", "istiod"),
			"HPA istio-system/istiod targets Deployment istiod, which " +
				"does not exist.",
			"targets Deployment istiod, which does not exist"},
		{node, "kwatch could not reach the kubelet on node 10-0-67-130 6 " +
			"times in the last 6 hours; node metrics for it are missing.",
			"has a kubelet that kwatch could not reach 6 times in the " +
				"last 6 hours"},
		{kube.KwatchSelf, "kwatch could not reach any of its 5 probed " +
			"dependencies; its own network may be restricted",
			"could not reach any of its 5 probed dependencies"},
		{inventory.CoreID(kube.KindExternalEndpoint, "", "db:5432"),
			"Endpoint did not answer within 3 seconds",
			"did not answer within 3 seconds"},
		{inventory.CoreID(kube.KindExternalEndpoint, "", "db:5432"),
			"Endpoint refused the connection", "refused the connection"},
		{inventory.CoreID(kube.KindExternalEndpoint, "", "db:5432"),
			"Endpoint name does not resolve",
			"has a name that does not resolve"},
		{inventory.CoreID(kube.KindExternalEndpoint, "", "db:5432"),
			"Endpoint DNS lookup failed", "has a DNS lookup that failed"},
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

// A custom resource keeps the kind spelling its CRD declares, which
// travels with the decision; one the API never declared stays in lower
// case, and commands are left as written.
func TestCustomKindKeepsDeclaredSpelling(t *testing.T) {
	agent := inventory.EntityID{Group: "datadoghq.com",
		Kind: kube.KindFor("DatadogAgent"), Namespace: "datadog",
		Name: "datadog"}
	names := map[inventory.Kind]string{agent.Kind: "DatadogAgent"}
	lead := []sentence{{part: partLead,
		text: capitalName(agent, shortName(agent)+" is failing.")}}
	if got := respell(lead, names)[0].text; got !=
		"DatadogAgent datadog is failing." {
		t.Fatalf("declared kind: %q", got)
	}
	step := []sentence{{part: partAction,
		text: "run kubectl get datadogagent datadog"}}
	if got := respell(step, names)[0].text; got != step[0].text {
		t.Fatalf("a command must stay as written: %q", got)
	}
	unknown := inventory.CoreID("widgetset", "shop", "w")
	got := capitalName(unknown, shortName(unknown)+" is failing.")
	if got != "Widgetset w is failing." {
		t.Fatalf("undeclared kind: %q", got)
	}
}
