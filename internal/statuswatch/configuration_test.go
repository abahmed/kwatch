package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestConfigurePolicyRejectsMalformedCondition(t *testing.T) {
	monitor := &Monitor{conditionRules: defaultConditionRules()}

	if err := monitor.ConfigurePolicy([]string{"Ready"}, nil); err == nil {
		t.Fatal("ConfigurePolicy accepted a malformed condition")
	}
	if !monitor.conditionRules["Ready"]["False"] {
		t.Fatal("malformed update replaced the existing condition rules")
	}
}

func TestConfigurePolicyTrimsConditionValues(t *testing.T) {
	monitor := &Monitor{}
	if err := monitor.ConfigurePolicy(
		[]string{" Ready = False "}, nil,
	); err != nil {
		t.Fatalf("ConfigurePolicy() returned error: %v", err)
	}
	if !monitor.conditionRules["Ready"]["False"] {
		t.Fatalf("condition rule was not normalized: %#v", monitor.conditionRules)
	}
}

func TestConfigurePolicyRejectsMalformedGraphReference(t *testing.T) {
	monitor := &Monitor{graphReferences: []graphReferenceRule{{
		path: []string{"spec", "service"},
		kind: "service",
	}}}

	if err := monitor.ConfigurePolicy(nil, []string{"=service"}); err == nil {
		t.Fatal("ConfigurePolicy accepted a malformed graph reference")
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
