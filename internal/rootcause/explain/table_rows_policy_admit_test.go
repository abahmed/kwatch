package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	podSecurityText = "Error creating: pods \"api-1-x\" is forbidden: " +
		"violates PodSecurity \"restricted:latest\": " +
		"allowPrivilegeEscalation != false"
	limitRangeText = "Error creating: pods \"api-1-x\" is forbidden: " +
		"maximum cpu usage per Container is 1, but limit is 2"
	policyText = "Error creating: pods \"api-1-x\" is forbidden: " +
		"ValidatingAdmissionPolicy 'no-latest.example.com' with " +
		"binding 'no-latest-binding' denied request: image tag latest"
)

func labelledNamespace(f *fixture, labels string) inventory.EntityID {
	ns := inventory.CoreID(kube.KindNamespace, "", "shop")
	f.observe(ns, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text(labels)})
	return ns
}

var policyAdmitRowCases = []rowCase{
	{row: "pod-security-rejects", want: "namespace//shop",
		build: func(f *fixture) inventory.EntityID {
			labelledNamespace(f, "pod-security.kubernetes.io/enforce="+
				"restricted")
			return createCase(f, podSecurityText)
		}},
	{row: "limit-range-rejects", want: "limitrange/shop/bounds",
		build: func(f *fixture) inventory.EntityID {
			lr := inventory.CoreID(kube.KindLimitRange, "shop", "bounds")
			f.add(lr)
			f.relate(lr, inventory.Constrains,
				inventory.CoreID(kube.KindNamespace, "", "shop"))
			return createCase(f, limitRangeText)
		}},
	{row: "admission-policy-denies",
		want: "validatingadmissionpolicy//no-latest.example.com",
		build: func(f *fixture) inventory.EntityID {
			f.add(inventory.CoreID(kindAdmissionPolicy, "",
				"no-latest.example.com"))
			return createCase(f, policyText)
		}},
}

// A check is blamed only when the error quotes it: a namespace without
// an enforce label, a policy the error does not name and an unrelated
// refusal blame nothing.
func TestPolicyAdmissionNeedsTheErrorToQuoteIt(t *testing.T) {
	cases := map[string]func(*fixture) inventory.EntityID{
		"no enforce label": func(f *fixture) inventory.EntityID {
			labelledNamespace(f, "team=shop")
			return createCase(f, podSecurityText)
		},
		"another policy": func(f *fixture) inventory.EntityID {
			f.add(inventory.CoreID(kindAdmissionPolicy, "", "other"))
			return createCase(f, policyText)
		},
		"limit range with another error": func(f *fixture) inventory.EntityID {
			lr := inventory.CoreID(kube.KindLimitRange, "shop", "bounds")
			f.add(lr)
			f.relate(lr, inventory.Constrains,
				inventory.CoreID(kube.KindNamespace, "", "shop"))
			return createCase(f, "forbidden: exceeded quota: compute")
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			effect := build(f)
			if c, ok := f.explain().CauseOf(effect); ok &&
				c.Row != "" && isPolicyAdmitRow(c.Row) {
				t.Fatalf("row %s blamed %s", c.Row, c.Root)
			}
		})
	}
}

func isPolicyAdmitRow(name string) bool {
	for _, row := range policyAdmitRows {
		if row.Name == name {
			return true
		}
	}
	return false
}
