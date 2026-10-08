package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func expiredSecret(
	sev detection.Severity, users ...string,
) (incident.Incident, []detection.Finding) {
	id := inventory.CoreID(kube.KindSecret, "shop", "shop-tls")
	f := detection.Finding{Entity: id, Reason: reasons.TLSCertExpired,
		Severity: sev}
	for _, u := range users {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceUsedBy, Value: u})
	}
	return incident.Incident{Root: id}, []detection.Finding{f}
}

func TestSecretStepNamesTheIngressThatUsesIt(t *testing.T) {
	p, members := expiredSecret(detection.Critical, "ingress/web")
	steps := nextSteps(p, members)
	if len(steps) != 1 || steps[0].Command !=
		"kubectl get ingress web -n shop" ||
		!strings.Contains(steps[0].Text, "hosts") ||
		strings.Contains(steps[0].Text, "pods") {
		t.Errorf("steps = %+v", steps)
	}
}

func TestSecretStepForSeveralIngressesListsThem(t *testing.T) {
	p, members := expiredSecret(detection.Critical,
		"ingress/a", "ingress/b")
	steps := nextSteps(p, members)
	if len(steps) != 1 ||
		steps[0].Command != "kubectl get ingress -n shop" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestSecretStepKeepsPodsWhenPodsUseIt(t *testing.T) {
	p, members := expiredSecret(detection.Critical, "pod/api-0")
	steps := nextSteps(p, members)
	if len(steps) != 1 || !strings.Contains(steps[0].Text, "pods") ||
		steps[0].Command != "kubectl describe secret shop-tls -n shop" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestSecretStepWhenNothingUsesIt(t *testing.T) {
	p, members := expiredSecret(detection.Warning)
	steps := nextSteps(p, members)
	if len(steps) != 1 || strings.Contains(steps[0].Text, "pods") ||
		!strings.Contains(steps[0].Text, "nothing") ||
		!strings.HasPrefix(steps[0].Command, "kubectl get ingress,pods "+
			"-n shop -o yaml | grep -c -F shop-tls") ||
		steps[0].Mutating {
		t.Errorf("steps = %+v", steps)
	}
}
