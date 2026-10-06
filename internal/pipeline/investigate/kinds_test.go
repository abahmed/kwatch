package investigate

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestInvestigatorPicksKindByRoot(t *testing.T) {
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1")
	container := kube.ContainerID("shop", "api-1", "app")
	tests := []struct {
		name string
		p    incident.Incident
		want string
	}{
		{"crash", incidentOf(pod, finding(container, "CrashLoop")),
			kindCrash},
		{"node beats crash", incidentOf(
			inventory.CoreID(kube.KindNode, "", "n1"),
			finding(container, "CrashLoop")), kindNode},
		{"scheduling", incidentOf(pod, finding(pod, "Unschedulable")),
			kindScheduling},
		{"config", incidentOf(
			inventory.CoreID(kube.KindConfigMap, "shop", "app"),
			finding(container, "CreateError.Config")), kindConfig},
		{"admission", incidentOf(
			inventory.CoreID(kube.KindValidatingHook, "", "policy"),
			finding(inventory.CoreID(kube.KindValidatingHook, "", "policy"),
				"Webhook.NoEndpoints")), kindAdmission},
		{"registry", incidentOf(pod, finding(container, "ImagePull")),
			kindRegistry},
	}
	r := NewInvestigator(Sources{Model: testModel(t)})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, ok := r.Plan(tt.p)
			if !ok || plan.Kind != tt.want {
				t.Fatalf("kind = %q (%v), want %q", plan.Kind, ok, tt.want)
			}
			if plan.Budget <= 0 || plan.Budget > MaxBudget {
				t.Fatalf("budget = %v, want within the pool deadline",
					plan.Budget)
			}
		})
	}
}

func TestInvestigatorSkipsIncidentsNoKindExplains(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "api")
	r := NewInvestigator(Sources{Model: testModel(t)})

	if _, ok := r.Plan(incidentOf(svc, finding(svc, "NoEndpoints"))); ok {
		t.Fatal("a Service without endpoints needs no investigation")
	}
	if _, ok := NewInvestigator(Sources{}).Plan(incidentOf(svc)); ok {
		t.Fatal("no model means no investigation")
	}
}

func TestInvestigateNodeReadsConditionsAndTopPods(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	var obs []inventory.Observation
	ready := kube.ConditionKey("Ready")
	obs = append(obs, observed(node, map[string]inventory.Value{
		ready:                               text("False"),
		ready + kube.AttrConditionReason:    text("KubeletNotReady"),
		kube.ConditionKey("MemoryPressure"): text("True"),
		kube.ConditionKey("DiskPressure"):   text("False"),
	}))
	usage := map[string][2]float64{
		"etl-0": {3 << 30, 200}, "api-1": {512 << 20, 1500},
		"idle": {0, 0}, "web-1": {1 << 30, 50}, "cache-0": {2 << 30, 10},
	}
	for name, u := range usage {
		pod := inventory.CoreID(kube.KindPod, "shop", name)
		c := kube.ContainerID("shop", name, "app")
		obs = append(obs, observed(pod, nil), related(pod, inventory.RunsOn, node),
			observed(c, map[string]inventory.Value{
				kube.AttrMemoryWorking: num(u[0]),
				kube.AttrCPUUsageMilli: num(u[1]),
			}), related(c, inventory.PartOf, pod))
	}
	s := Sources{Model: testModel(t, obs...)}

	_, r := planAndRun(t, s, incidentOf(node, finding(node, "NotReady")))

	want := map[string]string{
		incident.FactNode: "Ready=False (KubeletNotReady), MemoryPressure",
		incident.FactTopMemory: "etl-0 (3.0 GiB), cache-0 (2.0 GiB), " +
			"web-1 (1.0 GiB)",
		incident.FactTopCPU: "api-1 (1.5 cores), etl-0 (200m CPU), " +
			"web-1 (50m CPU)",
	}
	for fact, text := range want {
		got := evidenceOf(r, fact)
		if len(got) != 1 || got[0].Text != text {
			t.Errorf("%s = %+v, want %q", fact, got, text)
		}
	}
}

func TestInvestigateSchedulingCountsBlockers(t *testing.T) {
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1")
	message := "0/5 nodes are available: 3 Insufficient memory, " +
		"2 node(s) had untolerated taint {gpu: true}. preemption: 0/5 " +
		"nodes are available: 5 Preemption is not helpful."
	p := incidentOf(pod, finding(pod, "Unschedulable",
		detection.Evidence{Label: "scheduler", Value: message}))

	_, r := planAndRun(t, Sources{Model: testModel(t)}, p)

	got := evidenceOf(r, incident.FactScheduler)
	want := "Insufficient memory on 3 of 5 nodes, had untolerated taint " +
		"on 2 of 5 nodes"
	if len(got) != 1 || got[0].Text != want {
		t.Fatalf("scheduler = %+v, want %q", got, want)
	}
}

func TestInvestigateConfigNamesKeysNeverValues(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app")
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1")
	container := kube.ContainerID("shop", "api-1", "app")
	model := testModel(t,
		observed(cm, nil), observed(pod, nil), observed(container, nil),
		related(container, inventory.PartOf, pod),
		related(pod, inventory.References, cm),
		changedFields(cm, "data.DB_HOST", "data.DB_PORT",
			"metadata.labels"))
	p := incidentOf(pod, finding(container, "CreateError.Config"))

	_, r := planAndRun(t, Sources{Model: model}, p)

	got := evidenceOf(r, incident.FactKeys)
	if len(got) != 1 || got[0].Text != "DB_HOST, DB_PORT" ||
		got[0].Subject != "configmap app" {
		t.Fatalf("keys = %+v, want DB_HOST, DB_PORT of configmap app", got)
	}
	for _, e := range r.Evidence {
		if strings.Contains(e.Text, "secret-value") {
			t.Fatalf("evidence %q quotes a value", e.Text)
		}
	}
}

func TestInvestigateAdmissionCountsEndpointsAndQuotesFailure(t *testing.T) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	svc := inventory.CoreID(kube.KindService, "policy", "hook")
	other := inventory.CoreID(kube.KindService, "policy", "unwatched")
	slice := inventory.CoreID(kube.KindEndpointSlice, "policy", "hook-x")
	deploy := inventory.CoreID(kube.KindReplicaSet, "shop", "api-7f")
	model := testModel(t,
		observed(hook, nil), observed(svc, nil),
		related(hook, inventory.Serves, svc, other),
		observed(slice, map[string]inventory.Value{
			kube.AttrEndpointsReady: num(0)}),
		related(slice, inventory.Backs, svc),
		observed(deploy, map[string]inventory.Value{
			kube.ConditionKey("ReplicaFailure") + kube.AttrConditionMessage: text(
				`Internal error occurred: failed calling webhook "policy"`),
		}))
	var asked []inventory.EntityID
	s := Sources{Model: model, Endpoints: func(
		_ context.Context, id inventory.EntityID,
	) (int, bool) {
		asked = append(asked, id)
		return 2, true
	}}
	p := incidentOf(hook, finding(hook, "Webhook.NoEndpoints"),
		finding(deploy, "ReplicaFailure"))

	_, r := planAndRun(t, s, p)

	endpoints := evidenceOf(r, incident.FactEndpoints)
	if len(endpoints) != 2 || endpoints[0].Subject != "hook" ||
		endpoints[0].Text != "0" || endpoints[1].Text != "2" {
		t.Fatalf("endpoints = %+v, want hook from the model, the other "+
			"from the API", endpoints)
	}
	if len(asked) != 1 || asked[0] != other {
		t.Fatalf("API asked for %v, want only the Service the model "+
			"lacks", asked)
	}
	failure := evidenceOf(r, incident.FactWebhook)
	if len(failure) != 1 || !strings.Contains(failure[0].Text,
		"failed calling webhook") {
		t.Fatalf("webhook = %+v, want the API server's failure", failure)
	}
}

func TestInvestigateRegistryClassifiesPullError(t *testing.T) {
	container := kube.ContainerID("shop", "api-1", "app")
	tests := map[string]string{
		"toomanyrequests: You have reached your pull rate limit": "rate-limit",
		"unauthorized: authentication required":                  "auth",
		"manifest unknown":                                       "image",
		"dial tcp 10.0.0.1:443: i/o timeout":                     "network",
		"x509: certificate signed by unknown authority":          "tls",
	}
	for message, class := range tests {
		model := testModel(t, observed(container,
			map[string]inventory.Value{kube.AttrMessage: text(message)}))
		p := incidentOf(container, finding(container, "ImagePull"))

		_, r := planAndRun(t, Sources{Model: model}, p)

		got := evidenceOf(r, incident.FactPull)
		if len(got) != 1 || got[0].Text != class {
			t.Errorf("%q: pull = %+v, want %q", message, got, class)
		}
	}
}
