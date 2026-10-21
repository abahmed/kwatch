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

// Certificate detects expired TLS certificates: Secrets and probed
// HTTPS endpoints. A certificate that has not expired yet is no finding.
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
	if remaining > 0 {
		// Look again at the moment it expires, not before.
		ctx.RecheckAfter(remaining)
		return nil
	}
	return []detection.Finding{expiredCertificate(ctx, e, notAfter)}
}

// expiredCertificate is the finding for a certificate past its end. One
// nothing is known to use breaks nothing: it is a warning that says so,
// and it waits for the digest. One in use is critical. One whose use
// cannot be told, because a kind that could use it is not synced, is a
// warning with the plain fact and no claim about its use: only a known
// user makes it an outage.
func expiredCertificate(
	ctx detection.Context, e inventory.Entity, notAfter time.Time,
) detection.Finding {
	use := certificateUse(ctx, e)
	if use == certificateUnused {
		return detection.Finding{
			Reason: reasons.TLSCertExpired, Severity: detection.Warning,
			Since: notAfter,
			Summary: "TLS certificate expired on " +
				notAfter.UTC().Format("2006-01-02") +
				" and no pod or Ingress mounts or references it",
		}
	}
	severity := detection.Critical
	if use == certificateUnknown {
		severity = detection.Warning
	}
	return detection.Finding{
		Reason: reasons.TLSCertExpired, Severity: severity,
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

// certUse is what is known about whether a certificate is used.
type certUse uint8

const (
	// certificateUsed: a probed endpoint serves it, or something
	// references the Secret.
	certificateUsed certUse = iota
	// certificateUnused: nothing references the Secret, and every kind
	// that could has been listed.
	certificateUnused
	// certificateUnknown: nothing references the Secret in the model,
	// but a kind that could is not synced, so the model may lack the
	// referrer.
	certificateUnknown
)

// certificateUse tells whether something serves or mounts the
// certificate: a probed endpoint serves it by definition, and a Secret
// is used once an Ingress, pod or other object references it. It is
// unused only when Ingresses and pods are both synced: a Secret read by
// name through the API is not seen at all, so even then it is the
// model's claim, worded as such.
func certificateUse(ctx detection.Context, e inventory.Entity) certUse {
	if e.ID.Kind != kube.KindSecret {
		return certificateUsed
	}
	if ctx.Model != nil && len(ctx.Model.Related(e.ID,
		inventory.References, inventory.Incoming)) > 0 {
		return certificateUsed
	}
	ingresses, pods := ctx.Synced(kube.KindIngress), ctx.Synced(kube.KindPod)
	if !ingresses || !pods {
		return certificateUnknown
	}
	return certificateUnused
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
	// away: what it referenced no longer matters. Namespace deletion
	// removes Secrets before pods. An object applied next to its pod is
	// absorbed by the incident's settling, not delayed here.
	if podFinished(e) || flag(e, kube.AttrDeleting) ||
		namespaceTerminating(ctx, e.ID.Namespace) {
		return nil
	}
	running := text(e, kube.AttrPhase) == "Running" &&
		flag(e, kube.AttrReady)
	missingReasons, missing := missingObjects(ctx, e)
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
			// It read the object when it started: it runs, so
			// nothing has failed.
			severity = detection.Info
			summary += "; the pod is running now"
		}
		out = append(out, detection.Finding{
			Reason: reason, Severity: severity, Advisory: running,
			Summary: summary, Evidence: evidence,
		})
	}
	return out
}

// missingObjects lists what a pod references that does not exist, by
// finding reason, with the reasons in the order they were met.
// References marked optional and kinds not yet synced are skipped.
func missingObjects(
	ctx detection.Context, pod inventory.Entity,
) ([]string, map[string][]string) {
	missing := make(map[string][]string)
	var order []string
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.References, inventory.Outgoing,
	) {
		reason := missingReason(id.Kind)
		if reason == "" || !ctx.Synced(id.Kind) || ctx.Model.Exists(id) ||
			kube.OptionalReference(pod, id) {
			continue
		}
		if missing[reason] == nil {
			order = append(order, reason)
		}
		missing[reason] = append(missing[reason], id.Name)
	}
	return order, missing
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
