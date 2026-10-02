package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ingressTLSSecrets reports spec.tls secrets that do not exist. Most
// controllers then serve their default certificate, so HTTPS clients see
// a certificate for the wrong host.
func ingressTLSSecrets(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !ctx.Synced(kube.KindSecret) {
		return nil
	}
	var missing []string
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Outgoing,
	) {
		if id.Kind == kube.KindSecret && id.Name != "" &&
			!ctx.Model.Exists(id) {
			missing = append(missing, id.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	evidence := make([]detection.Evidence, 0, len(missing))
	for _, name := range missing {
		evidence = append(evidence,
			detection.Evidence{Label: "tls secret", Value: name})
	}
	return []detection.Finding{{
		Reason: reasons.IngressTLSSecretMissing, Severity: detection.Warning,
		Summary: "Ingress TLS Secret " + strings.Join(missing, ", ") +
			" does not exist; HTTPS serves the controller's default " +
			"certificate",
		Evidence: evidence,
	}}
}

// ingressClass reports an Ingress whose ingressClassName names an
// IngressClass that does not exist: no controller serves it. An Ingress
// without a class name is not judged, since controllers may serve it by
// the legacy annotation or as the default.
func ingressClass(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	name := text(e, kube.AttrIngressClass)
	if ctx.Model == nil || name == "" || !ctx.Synced(kube.KindIngressClass) {
		return nil
	}
	class := inventory.CoreID(kube.KindIngressClass, "", name)
	if ctx.Model.Exists(class) {
		return nil
	}
	return []detection.Finding{{
		Reason:   reasons.IngressClassMissing,
		Severity: detection.Warning, Health: detection.Failing,
		Summary: "IngressClass " + name + " does not exist; no " +
			"controller serves this Ingress",
		Evidence: []detection.Evidence{{Label: "ingress class", Value: name}},
	}}
}
