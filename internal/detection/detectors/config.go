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
const DefaultCertificateWarning = 2 * format.Week

// Certificate detects expired and soon-expiring TLS Secrets.
type Certificate struct{}

// Name implements detection.Detector.
func (Certificate) Name() string { return "certificate" }

// Kinds implements detection.Detector.
func (Certificate) Kinds() []inventory.Kind {
	// A probed HTTPS endpoint carries the expiry of the certificate it
	// served, so the check covers what clients see too.
	return []inventory.Kind{kube.KindSecret, kube.KindEndpoint}
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
		return []detection.Finding{expiredCertificate(ctx, e, notAfter)}
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

// expiredCertificate is the finding for a certificate past its end. One
// nobody uses breaks nothing: it is a warning that says so, and it
// waits for the digest. One in use is critical.
func expiredCertificate(
	ctx detection.Context, e inventory.Entity, notAfter time.Time,
) detection.Finding {
	if !certificateInUse(ctx, e) {
		return detection.Finding{
			Reason: reasons.TLSCertExpired, Severity: detection.Warning,
			Since: notAfter,
			Summary: "TLS certificate expired on " +
				notAfter.UTC().Format("2006-01-02") +
				" and nothing in the cluster references it",
		}
	}
	return detection.Finding{
		Reason: reasons.TLSCertExpired, Severity: detection.Critical,
		Since: notAfter,
		Summary: "TLS certificate expired " +
			format.Duration(ctx.Now.Sub(notAfter)) + " ago",
		Evidence: certificateUsers(ctx, e),
	}
}

// certificateUsers names what references the Secret, so the next steps
// can point at the real user (an Ingress, not "the pods").
func certificateUsers(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	if e.ID.Kind != kube.KindSecret || ctx.Model == nil {
		return nil
	}
	var out []detection.Evidence
	for _, user := range ctx.Model.Related(e.ID, inventory.References,
		inventory.Incoming) {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceUsedBy,
			Value: string(user.Kind) + "/" + user.Name})
	}
	return out
}

// certificateInUse reports whether something serves or mounts the
// certificate: a probed endpoint serves it by definition, and a Secret
// counts once an Ingress, pod or other object references it.
func certificateInUse(ctx detection.Context, e inventory.Entity) bool {
	if e.ID.Kind != kube.KindSecret {
		return true
	}
	return ctx.Model != nil && len(ctx.Model.Related(e.ID,
		inventory.References, inventory.Incoming)) > 0
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
	// A finished pod never starts again and a deleting one is going
	// away: what it referenced no longer matters.
	if podFinished(e) || flag(e, kube.AttrDeleting) {
		return nil
	}
	running := text(e, kube.AttrPhase) == "Running" &&
		flag(e, kube.AttrReady)
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
		severity := detection.Critical
		summary := "Pod references " + strings.Join(names, ", ") +
			", which does not exist"
		if running {
			// It read the object when it started; only a restart fails.
			severity = detection.Warning
			summary += "; the pod runs now but will fail on its next restart"
		}
		out = append(out, detection.Finding{
			Reason: reason, Severity: severity,
			Summary: summary, Evidence: evidence,
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
