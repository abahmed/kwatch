package detect

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultCertificateWarning is how far ahead an expiring TLS certificate
// is reported.
const DefaultCertificateWarning = 14 * 24 * time.Hour

// Certificate detects expired and soon-expiring TLS Secrets.
type Certificate struct{}

// Name implements signal.Detector.
func (Certificate) Name() string { return "certificate" }

// Kinds implements signal.Detector.
func (Certificate) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindSecret}
}

// Detect implements signal.Detector.
func (Certificate) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	notAfter := timestamp(e, kube.AttrCertExpiry)
	if notAfter.IsZero() {
		return nil
	}
	remaining := notAfter.Sub(ctx.Now)
	if remaining <= 0 {
		return []signal.Signal{{
			Reason: constant.ReasonTLSCertExpired, Severity: signal.Critical,
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
	return []signal.Signal{{
		Reason: constant.ReasonTLSCertExpiringSoon, Severity: signal.Warning,
		Since:   notAfter.Add(-DefaultCertificateWarning),
		Summary: "TLS certificate expires in " + format.Duration(remaining),
	}}
}

// Missing detects references to objects that do not exist: a pod that
// needs a Secret, ConfigMap or ServiceAccount nobody created.
type Missing struct{}

// Name implements signal.Detector.
func (Missing) Name() string { return "missing-reference" }

// Kinds implements signal.Detector.
func (Missing) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindPod}
}

// Detect implements signal.Detector. Missing objects of one kind share a
// signal, since the signal key is the reason.
func (Missing) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	missing := make(map[string][]string)
	var reasons []string
	for _, id := range ctx.Model.Related(
		e.ID, knowledge.References, knowledge.Outgoing,
	) {
		reason := missingReason(id.Kind)
		if reason == "" || !ctx.Synced(id.Kind) || ctx.Model.Exists(id) {
			continue
		}
		if missing[reason] == nil {
			reasons = append(reasons, reason)
		}
		missing[reason] = append(missing[reason], id.Name)
	}
	out := make([]signal.Signal, 0, len(reasons))
	for _, reason := range reasons {
		names := missing[reason]
		evidence := make([]signal.Evidence, 0, len(names))
		for _, name := range names {
			evidence = append(evidence,
				signal.Evidence{Label: "missing", Value: name})
		}
		out = append(out, signal.Signal{
			Reason: reason, Severity: signal.Critical,
			Summary: "Pod references " + strings.Join(names, ", ") +
				", which does not exist",
			Evidence: evidence,
		})
	}
	return out
}

func missingReason(kind knowledge.Kind) string {
	switch kind {
	case kube.KindSecret:
		return constant.ReasonProjectedSecretMissing
	case kube.KindConfigMap:
		return constant.ReasonProjectedConfigMapMissing
	case kube.KindAccount:
		return constant.ReasonServiceAccountMissing
	default:
		return ""
	}
}
