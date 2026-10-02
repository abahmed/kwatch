package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultCertificateWarning is how far ahead an expiring TLS certificate
// is reported.
const DefaultCertificateWarning = 14 * 24 * time.Hour

// Certificate detects expired and soon-expiring TLS Secrets.
type Certificate struct{}

// Name implements detection.Detector.
func (Certificate) Name() string { return "certificate" }

// Kinds implements detection.Detector.
func (Certificate) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindSecret}
}

// Detect implements detection.Detector.
func (Certificate) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	notAfter := timestamp(e, kube.AttrCertExpiry)
	if notAfter.IsZero() {
		return nil
	}
	remaining := notAfter.Sub(ctx.Now)
	if remaining <= 0 {
		return []detection.Finding{{
			Reason: reasons.TLSCertExpired, Severity: detection.Critical,
			Since: notAfter,
			Summary: "TLS certificate expired " +
				format.Duration(-remaining) + " ago",
		}}
	}
	if remaining > DefaultCertificateWarning {
		ctx.RecheckAfter(remaining - DefaultCertificateWarning)
		return nil
	}
	ctx.RecheckAfter(remaining)
	return []detection.Finding{{
		Reason: reasons.TLSCertExpiringSoon, Severity: detection.Warning,
		Since:   notAfter.Add(-DefaultCertificateWarning),
		Summary: "TLS certificate expires in " + format.Duration(remaining),
	}}
}

// Missing detects references to objects that do not exist: a pod that
// needs a Secret, ConfigMap or ServiceAccount nobody created. References
// marked optional are skipped: the pod starts without them.
type Missing struct{}

// Name implements detection.Detector.
func (Missing) Name() string { return "missing-reference" }

// Kinds implements detection.Detector.
func (Missing) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

// Detect implements detection.Detector. Missing objects of one kind share a
// finding, since the finding key is the reason.
func (Missing) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	missing := make(map[string][]string)
	var missingReasons []string
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Outgoing,
	) {
		reason := missingReason(id.Kind)
		if reason == "" || !ctx.Synced(id.Kind) || ctx.Model.Exists(id) ||
			kube.OptionalReference(e, id) {
			continue
		}
		if missing[reason] == nil {
			missingReasons = append(missingReasons, reason)
		}
		missing[reason] = append(missing[reason], id.Name)
	}
	out := make([]detection.Finding, 0, len(missingReasons))
	for _, reason := range missingReasons {
		names := missing[reason]
		evidence := make([]detection.Evidence, 0, len(names))
		for _, name := range names {
			evidence = append(evidence,
				detection.Evidence{Label: "missing", Value: name})
		}
		out = append(out, detection.Finding{
			Reason: reason, Severity: detection.Critical,
			Summary: "Pod references " + strings.Join(names, ", ") +
				", which does not exist",
			Evidence: evidence,
		})
	}
	return out
}

func missingReason(kind inventory.Kind) string {
	switch kind {
	case kube.KindSecret:
		return reasons.ProjectedSecretMissing
	case kube.KindConfigMap:
		return reasons.ProjectedConfigMapMissing
	case kube.KindAccount:
		return reasons.ServiceAccountMissing
	case kube.KindRuntimeClass:
		return reasons.RuntimeClassMissing
	default:
		return ""
	}
}
