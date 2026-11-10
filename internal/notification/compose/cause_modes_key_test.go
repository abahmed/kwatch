package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A Secret the kubelet named as lacking a key exists: it is never
// worded "does not exist".
func TestSecretLackingAKeyIsNotSaidToBeMissing(t *testing.T) {
	root := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	for _, rule := range []string{"config-missing-or-changed",
		"configmap-missing-or-changed"} {
		cause := rootcause.CauseRecord{Rule: rule,
			Mode: explain.ModeKeyMissing, Root: root}
		if got := causeWordsFor(&cause); got !=
			"lacks a key the pods read" {
			t.Errorf("%s: worded %q", rule, got)
		}
	}
}
