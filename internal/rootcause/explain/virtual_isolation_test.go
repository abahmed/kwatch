package explain

import (
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var isolationRowCases = []rowCase{
	{row: RulePolicyIsolatesBackend,
		want: "networkpolicy/policy-system/default-deny",
		build: func(f *fixture) inventory.EntityID {
			hook := inventory.CoreID(kube.KindValidatingHook, "", "guard")
			svc := inventory.CoreID(kube.KindService, "policy-system", "guard")
			pod := inventory.CoreID(kube.KindPod, "policy-system", "guard-0")
			f.observe(hook, map[string]inventory.Value{
				kube.AttrWebhookNames: inventory.Text("v.example")})
			f.relate(hook, inventory.Serves, svc)
			f.observe(pod, map[string]inventory.Value{
				kube.AttrLabels: inventory.Text("app=guard")})
			f.observe(svc, map[string]inventory.Value{
				kube.AttrSelector: inventory.Text("app=guard")})
			policy := addPolicy(f, "policy-system", "default-deny",
				networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.
					PolicyType{networkingv1.PolicyTypeIngress}})
			f.change(policy, 1)
			return createCase(f, `failed calling webhook "v.example"`)
		}},
}
