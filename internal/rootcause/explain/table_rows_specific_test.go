package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var workloadRowCases = []rowCase{
	{row: "rollout", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 2)
			f.change(inventory.CoreID(kube.KindDeployment, "shop", "api"),
				1, "spec.template.spec.containers[0].image")
			for _, pod := range pods {
				f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
			}
			return containerOf(pods[0])
		}},
	{row: "config-missing-or-changed", want: "secret/shop/db-creds",
		build: func(f *fixture) inventory.EntityID {
			return usesCase(f, kube.KindSecret, true)
		}},
	{row: "configmap-missing-or-changed", want: "configmap/shop/db-creds",
		build: func(f *fixture) inventory.EntityID {
			return usesCase(f, kube.KindConfigMap, false)
		}},
	{row: "claim-not-usable", want: "persistentvolumeclaim/data/vol",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("data", "db", 1)
			pvc := inventory.CoreID(kube.KindPVC, "data", "vol")
			f.add(pvc)
			f.relate(pods[0], inventory.Mounts, pvc)
			f.fail(pvc, "ClaimFailed", degradedH, 1, "")
			f.fail(pods[0], "Pending", degradedH, 2, "")
			return pods[0]
		}},
	{row: "policy-restricts", want: "networkpolicy/pay/deny",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("pay", "api", 2)
			policy := inventory.CoreID(kube.KindNetworkPolicy, "pay", "deny")
			f.add(policy)
			f.change(policy, 1)
			for _, pod := range pods {
				f.links[policy] = append(f.links[policy],
					Link{Type: inventory.Selects, To: pod})
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					"dial tcp 10.0.0.9:5432: i/o timeout")
			}
			return containerOf(pods[0])
		}},
}

// usesCase makes a workload's pods fail with a config error naming a
// secret or config map that changed or does not exist.
func usesCase(
	f *fixture, kind inventory.Kind, changed bool,
) inventory.EntityID {
	pods := f.workload("shop", "api", 2)
	ref := inventory.CoreID(kind, "shop", "db-creds")
	if changed {
		f.add(ref)
		f.change(ref, 1, "data.password")
	}
	for _, pod := range pods {
		f.relate(pod, inventory.References, ref)
		f.fail(containerOf(pod), "CreateError.Config", failingH, 2,
			"couldn't find key password in "+string(kind)+" shop/db-creds")
	}
	return containerOf(pods[0])
}

var clusterRowCases = []rowCase{
	{row: "claim-pins-pod", want: "persistentvolumeclaim/streaming/data-kafka-0",
		build: func(f *fixture) inventory.EntityID {
			pod, _ := pinnedPod(f)
			return pod
		}},
	{row: "registry-refuses", want: "registry//registry.corp.example",
		build: func(f *fixture) inventory.EntityID {
			var first inventory.EntityID
			for _, name := range []string{"api", "web"} {
				pod := f.workload("shop", name, 1)[0]
				image := inventory.CoreID(kube.KindImage, "",
					"registry.corp.example/shop/"+name+":1")
				f.relate(containerOf(pod), inventory.Pulls, image)
				f.note(pod, "failed to authorize: 401 Unauthorized")
				f.fail(containerOf(pod), "ImagePull", failingH, 2, "")
				if first.IsZero() {
					first = containerOf(pod)
				}
			}
			return first
		}},
	{row: "cluster-dns-failing", want: "cluster-dns//cluster-dns",
		build: func(f *fixture) inventory.EntityID {
			f.fail(kube.ClusterDNS, "Unavailable.CoreDNS", failingH, 1, "")
			pods := f.workload("shop", "cart", 2)
			for _, pod := range pods {
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					"lookup db.shop.svc: no such host")
			}
			return containerOf(pods[0])
		}},
	{row: "cluster-dns-servers", want: "cluster-dns//cluster-dns",
		build: func(f *fixture) inventory.EntityID {
			servers := f.workload("kube-system", "coredns", 2)
			for _, pod := range servers {
				f.fail(containerOf(pod), "CrashLoop", failingH, 1,
					"plugin/forward: no upstream")
			}
			client := f.workload("shop", "cart", 1)[0]
			f.fail(containerOf(client), "CrashLoop", failingH, 2,
				"lookup db.shop.svc: no such host")
			return containerOf(servers[0])
		}},
	{row: "webhook-rejects", want: "validatingwebhookconfiguration//policy",
		build: func(f *fixture) inventory.EntityID {
			hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
			f.observe(hook, map[string]inventory.Value{
				kube.AttrWebhookNames: inventory.Text("v.example")})
			f.fail(hook, "Webhook.NoEndpoints", failingH, 1, "")
			return createCase(f, `failed calling webhook "v.example"`)
		}},
	{row: "service-no-endpoints", want: "service/policy/hook",
		build: func(f *fixture) inventory.EntityID {
			service := inventory.CoreID(kube.KindService, "policy", "hook")
			f.add(service)
			return webhooksBehind(f, service, "Webhook.NoEndpoints",
				"policy", "mutate")
		}},
	{row: "webhook-backend-missing", want: "service/kyverno/kyverno-svc",
		build: func(f *fixture) inventory.EntityID {
			service := inventory.CoreID(kube.KindService, "kyverno",
				"kyverno-svc")
			return webhooksBehind(f, service, "Webhook.BackendMissing",
				"cleanup", "exception")
		}},
	{row: "metrics-api-down", want: "apiservice//v1beta1.metrics.k8s.io",
		build: func(f *fixture) inventory.EntityID {
			api := inventory.CoreID(kube.KindAPIService, "",
				"v1beta1.metrics.k8s.io")
			hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
			f.add(api, hpa)
			f.fail(api, "APIServiceUnavailable", degradedH, 1, "")
			f.fail(hpa, "Scaling.NoMetrics", degradedH, 2, "")
			return hpa
		}},
	{row: "quota-exhausted", want: "resourcequota/shop/compute",
		build: func(f *fixture) inventory.EntityID {
			quota := inventory.CoreID(kube.KindQuota, "shop", "compute")
			f.add(quota)
			f.relate(quota, inventory.Constrains,
				inventory.CoreID(kube.KindNamespace, "", "shop"))
			f.fail(quota, "QuotaExhausted", degradedH, 1, "")
			return createCase(f, "forbidden: exceeded quota: compute")
		}},
	{row: "scheduler-capacity", want: "scheduling//Insufficient memory",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "etl", 2)
			for _, pod := range pods {
				f.findings[pod] = append(f.findings[pod], detection.Finding{
					Entity: pod, Mode: "Unschedulable", Health: degradedH,
					Since: t0, Evidence: []detection.Evidence{{
						Label: "scheduler", Value: "0/3 nodes are " +
							"available: 3 Insufficient memory."}},
				})
			}
			return pods[0]
		}},
	{row: "zone-failing", want: "zone//zone-b",
		build: func(f *fixture) inventory.EntityID {
			f.nodes("zone-a", "a1")
			nodes := f.nodes("zone-b", "b1", "b2")
			for _, node := range nodes {
				f.fail(node, "NotReady", failingH, 1, "")
			}
			return nodes[0]
		}},
	{row: "nodepool-failing", want: "nodepool//gpu",
		build: func(f *fixture) inventory.EntityID {
			healthy := f.nodes("zone-a", "c1")
			f.relate(healthy[0], inventory.PartOf,
				inventory.CoreID(kube.KindNodePool, "", "cpu"))
			nodes := f.nodes("zone-a", "g1", "g2")
			for _, node := range nodes {
				f.relate(node, inventory.PartOf,
					inventory.CoreID(kube.KindNodePool, "", "gpu"))
				f.fail(node, "NotReady", failingH, 1, "")
			}
			return nodes[0]
		}},
}

// createCase makes two ReplicaSets fail to create pods with text.
func createCase(f *fixture, text string) inventory.EntityID {
	var first inventory.EntityID
	for _, name := range []string{"api", "web"} {
		f.workload("shop", name, 0)
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name+"-1")
		f.fail(rs, "FailedCreate", failingH, 2, text)
		if first.IsZero() {
			first = rs
		}
	}
	return first
}

// TestTableRows solves one fixture per row and checks that the row
// links the expected root to the failure.
func TestTableRows(t *testing.T) {
	groups := [][]rowCase{genericRowCases, nodeRowCases, sharedRowCases,
		workloadRowCases, clusterRowCases, controlPlaneRowCases,
		accessRowCases, trafficRowCases, lifecycleRowCases,
		workloadConfigRowCases, calledRowCases, scalingRowCases,
		agentRowCases, serviceCallRowCases}
	tested := map[string]bool{}
	for _, group := range groups {
		for _, tc := range group {
			t.Run(tc.row, func(t *testing.T) {
				f := newFixture(t)
				effect := tc.build(f)
				c := requireCause(t, f.explain(), effect, tc.want)
				if c.Row != tc.row {
					t.Fatalf("row = %s, want %s (evidence %v)", c.Row,
						tc.row, c.Contributions)
				}
			})
			tested[tc.row] = true
		}
	}
	for _, row := range Table() {
		if !tested[row.Name] {
			t.Errorf("row %s has no fixture test", row.Name)
		}
	}
}
