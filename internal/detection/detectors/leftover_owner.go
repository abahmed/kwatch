package detectors

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leftBehindBy reports that member belongs with the absent backend
// service of the webhook config, run by controller (the Service itself
// when no workload is left). A namespace holds the objects of many
// installations, so being there is not enough; one of these is:
//   - member is the webhook, the Service or the controller;
//   - member and one of them carry the same installation name
//     (app.kubernetes.io/instance or part-of label, Helm release);
//   - member is named after the controller or the Service, without
//     its role word ("policy-tls" beside "policy-webhook");
//   - member is the Secret cert-manager injects the webhook's CA from.
func leftBehindBy(
	ctx detection.Context,
	member, config, service, controller inventory.EntityID,
) bool {
	backend := []inventory.EntityID{config, service, controller}
	if slices.Contains(backend, member) {
		return true
	}
	e, ok := ctx.Model.Entity(member)
	if !ok {
		return false
	}
	for _, id := range backend {
		if other, ok := ctx.Model.Entity(id); ok && sameRelease(e, other) {
			return true
		}
	}
	return namedAfter(member.Name, controller.Name) ||
		namedAfter(member.Name, service.Name) ||
		caSecretOf(ctx, e, config)
}

// sameRelease reports two objects that name a common installation.
func sameRelease(a, b inventory.Entity) bool {
	names := releaseNames(b)
	for _, name := range releaseNames(a) {
		if slices.Contains(names, name) {
			return true
		}
	}
	return false
}

// releaseNames are the installation names e carries.
func releaseNames(e inventory.Entity) []string {
	raw := text(e, kube.AttrRelease)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// roleWords end the names of the parts an operator installs; the stem
// before them names the operator.
var roleWords = []string{
	"-webhook-service", "-webhook-server", "-webhook", "-controller",
	"-operator", "-manager", "-service", "-svc", "-server",
}

// minStem is the shortest stem that names an operator: a shorter one
// ("my", "app") is shared by unrelated objects.
const minStem = 4

// namedAfter reports a name that starts with owner, or with owner
// without its role word, followed by a separator: "policy-tls" and
// "policy-controller-cert" are named after "policy-controller".
func namedAfter(name, owner string) bool {
	stems := []string{owner}
	for _, word := range roleWords {
		if stem, ok := strings.CutSuffix(owner, word); ok {
			stems = append(stems, stem)
		}
	}
	for _, stem := range stems {
		if len(stem) < minStem {
			continue
		}
		if rest, ok := strings.CutPrefix(name, stem); ok &&
			(rest == "" || rest[0] == '-' || rest[0] == '.') {
			return true
		}
	}
	return false
}

// caSecretOf reports the Secret that cert-manager injects the webhook's
// CA bundle from, directly or through the Certificate that writes it.
func caSecretOf(
	ctx detection.Context, secret inventory.Entity,
	config inventory.EntityID,
) bool {
	if secret.ID.Kind != kube.KindSecret {
		return false
	}
	hook, ok := ctx.Model.Entity(config)
	if !ok {
		return false
	}
	from := text(hook, kube.AttrCAFrom)
	switch {
	case from == "":
		return false
	case from == "secret:"+secret.ID.Namespace+"/"+secret.ID.Name:
		return true
	}
	cert := text(secret, kube.AttrCertificateName)
	return cert != "" && from == "certificate:"+cert
}
