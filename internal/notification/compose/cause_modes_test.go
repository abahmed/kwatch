package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A Secret or ConfigMap blamed for being edited exists: the words follow
// the mode that matched, whichever row blamed it.
func TestCauseWordsFollowTheModeOfConfigRows(t *testing.T) {
	cart := inventory.CoreID(kube.KindDeployment, "shop", "cart")
	rows := map[string]inventory.Kind{
		"config-missing-or-changed":        kube.KindSecret,
		"configmap-missing-or-changed":     kube.KindConfigMap,
		"run-secret-missing-or-changed":    kube.KindSecret,
		"run-configmap-missing-or-changed": kube.KindConfigMap,
	}
	modes := []struct {
		mode detection.Mode
		want string
	}{
		{explain.ModeMissing, "does not exist"},
		{explain.ModeChanged, "changed shortly before"},
		{explain.ModeSpecChanged, "changed shortly before"},
		{explain.ModeDeleted, "does not exist"},
		{"", "does not exist"},
	}
	for rule, kind := range rows {
		root := inventory.CoreID(kind, "shop", "app-config")
		for _, m := range modes {
			cause := rootcause.CauseRecord{Rule: rule, Mode: m.mode,
				Root: root}
			got := causePhrase(&cause, cart)
			want := shortName(root) + " " + m.want
			if got != want {
				t.Errorf("%s/%q: %q, want %q", rule, m.mode, got, want)
			}
		}
	}
}

func TestChangedConfigWithNoChangeNeverSaysItDoesNotExist(t *testing.T) {
	root := inventory.CoreID(kube.KindSecret, "shop", "db")
	cause := rootcause.CauseRecord{Rule: "config-missing-or-changed",
		Mode: explain.ModeChanged, Root: root}
	if got := causeWordsFor(&cause); got == "does not exist" {
		t.Fatalf("an edited secret was worded %q", got)
	}
}
