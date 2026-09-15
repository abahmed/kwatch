package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSetConditionRulesRejectsMalformedEntry(t *testing.T) {
	monitor := &Monitor{conditionRules: defaultConditionRules()}

	if err := monitor.SetConditionRules([]string{"Ready"}); err == nil {
		t.Fatal("SetConditionRules accepted a malformed rule")
	}
	if !monitor.conditionRules["Ready"]["False"] {
		t.Fatal("malformed update replaced the existing condition rules")
	}
}

func TestSetConditionRulesTrimsValues(t *testing.T) {
	monitor := &Monitor{}
	if err := monitor.SetConditionRules([]string{" Ready = False "}); err != nil {
		t.Fatalf("SetConditionRules() returned error: %v", err)
	}
	if !monitor.conditionRules["Ready"]["False"] {
		t.Fatalf("condition rule was not normalized: %#v", monitor.conditionRules)
	}
}

func TestSetGraphReferenceRulesRejectsMalformedEntry(t *testing.T) {
	monitor := &Monitor{graphReferences: []graphReferenceRule{{
		path: []string{"spec", "service"},
		kind: "service",
	}}}

	if err := monitor.SetGraphReferenceRules([]string{"=service"}); err == nil {
		t.Fatal("SetGraphReferenceRules accepted a malformed rule")
	}
	if len(monitor.graphReferences) != 1 ||
		monitor.graphReferences[0].kind != "service" {
		t.Fatalf("malformed update replaced graph references: %#v",
			monitor.graphReferences)
	}
}

func TestVersionKeyKeepsNamespaceAndResourceIdentity(t *testing.T) {
	key := versionKey(schema.GroupVersionResource{
		Group: "apps.example.com", Version: "v1", Resource: "widgets",
	}, "team-a")
	if key != "apps.example.com/v1, Resource=widgets|team-a" {
		t.Fatalf("versionKey() = %q", key)
	}
}
