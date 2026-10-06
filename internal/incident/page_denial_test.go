package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// TestWebhookDenialNotifiesButTimeoutPages: a fail-closed webhook that
// answers and denies one workload's pods is a policy decision (notify);
// one that times out blocks every create (page).
func TestWebhookDenialNotifiesButTimeoutPages(t *testing.T) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	rs := entity(kube.KindReplicaSet, "web-1")
	cases := map[string]struct {
		mode detection.Mode
		want Tier
	}{
		"timeout pages":     {explain.ModeWebhookTimeout, Page},
		"call failure":      {explain.ModeWebhookCallFailed, Page},
		"denial notifies":   {explain.ModeWebhookDenied, Notify},
		"no endpoints page": {detection.ModeWebhookNoEndpoints, Page},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			observe(t, model, hook,
				map[string]string{kube.AttrFailurePolicy: "Fail"})
			p := incidentOf(hook, nil,
				sig(rs, "FailedCreate", detection.Warning))
			p.Cause = &rootcause.CauseRecord{Root: hook,
				Rule: webhookRejectsRule, Mode: c.mode}
			p.admissionBlocked = admissionBlocked(model, p)
			if got := tier(p); got != c.want {
				t.Fatalf("tier = %v, want %v", got, c.want)
			}
		})
	}
}
