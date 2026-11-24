package incident

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leftoverClue is what one low finding says about the controller that
// left its object behind (leftover_link.go).
type leftoverClue struct {
	key    detection.Key
	entity inventory.EntityID
	// anchor is the gone or stopped controller the finding names, a
	// root for the group: the backend a detector marked, the target an
	// autoscaler lost, the release stuck uninstalling, or, for a parked
	// workload, its namespace. Zero when the finding names none.
	anchor inventory.EntityID
	// scope is the namespace names are matched in.
	scope string
	// names are the names the finding knows its controller by: the
	// anchor's own and its installation's, or, without an anchor, the
	// object's (an expired certificate) or the labels a budget selects.
	names []string
	// domains are the API domains of the controller: of the webhooks
	// calling its backend, or of the finalizers an object waits on.
	domains []string
}

// clueOf is the clue of finding f; ok is false for a finding that is
// not low alone or says nothing about a removed controller.
func clueOf(
	model inventory.Reader, f detection.Finding,
) (leftoverClue, bool) {
	if f.Advisory || !lowAlone(f) {
		return leftoverClue{}, false
	}
	c := leftoverClue{key: f.Key(), entity: f.Entity,
		scope: f.Entity.Namespace}
	switch {
	case !f.Leftover.IsZero():
		c.anchor, c.scope = f.Leftover, f.Leftover.Namespace
		c.names = anchorNames(model, f.Leftover)
		c.domains = webhookDomains(model, f.Entity)
	case f.Reason == reasons.HPATargetMissing:
		c.anchor = lostScaleTarget(model, f.Entity)
		c.names = []string{c.anchor.Name}
	case f.Reason == reasons.HelmReleaseStuck:
		c.anchor, c.names = f.Entity, []string{f.Entity.Name}
	case f.Reason == reasons.ScaledToZeroRouted:
		c.anchor = inventory.CoreID(kube.KindNamespace, "",
			f.Entity.Namespace)
	case f.Reason == reasons.PdbSelectsNothing:
		c.names = selectorValues(textAttr(model, f.Entity,
			kube.AttrSelector))
	case f.Reason == reasons.TLSCertExpired:
		c.names = []string{f.Entity.Name}
	case isStuckDeletion(f):
		c.domains = finalizerDomains(f)
	}
	return c, !c.anchor.IsZero() || len(c.names) > 0 ||
		len(c.domains) > 0
}

// lostScaleTarget is the scale target of an autoscaler that does not
// exist; zero when every target does.
func lostScaleTarget(
	model inventory.Reader, hpa inventory.EntityID,
) inventory.EntityID {
	for _, target := range model.Related(hpa, inventory.Scales,
		inventory.Outgoing) {
		if !model.Exists(target) {
			return target
		}
	}
	return inventory.EntityID{}
}

// anchorNames are the names of a controller: its own, the labels its
// Service selects pods by and the installations it belongs to.
func anchorNames(
	model inventory.Reader, anchor inventory.EntityID,
) []string {
	names := []string{anchor.Name}
	if anchor.Kind == kube.KindService {
		names = append(names, selectorValues(textAttr(model, anchor,
			kube.AttrSelector))...)
	}
	return append(names, splitMarks(textAttr(model, anchor,
		kube.AttrRelease))...)
}

// selectorValues are the values of a selector's equality terms:
// "app=mesh,tier=control" selects pods by "mesh" and "control".
func selectorValues(selector string) []string {
	var out []string
	for _, term := range strings.Split(selector, ",") {
		key, value, ok := strings.Cut(term, "=")
		if !ok || strings.HasSuffix(key, "!") || value == "" {
			continue
		}
		out = append(out, strings.TrimPrefix(value, "="))
	}
	return out
}

// webhookDomains are the API domains of a webhook configuration's
// webhooks: "mutate-runner.runs.example.com" serves runs.example.com.
func webhookDomains(
	model inventory.Reader, id inventory.EntityID,
) []string {
	if id.Kind != kube.KindMutatingWebhook &&
		id.Kind != kube.KindValidatingHook {
		return nil
	}
	var out []string
	for _, name := range splitMarks(textAttr(model, id,
		kube.AttrWebhookNames)) {
		if _, domain, ok := strings.Cut(name, "."); ok {
			out = appendDomain(out, domain)
		}
	}
	return out
}

// finalizerDomains are the API domains of the finalizers a stuck
// deletion waits on: "runs.example.com/cleanup" is runs.example.com's.
func finalizerDomains(f detection.Finding) []string {
	var out []string
	for _, name := range finalizerNames(f) {
		if domain, _, ok := strings.Cut(name, "/"); ok {
			out = appendDomain(out, domain)
		}
	}
	return out
}

// appendDomain adds domain unless Kubernetes itself owns it, or it is
// too short to name one API: every core finalizer and webhook shares
// those.
func appendDomain(out []string, domain string) []string {
	if strings.Count(domain, ".") < 1 || domain == "kubernetes.io" ||
		strings.HasSuffix(domain, ".kubernetes.io") ||
		strings.HasSuffix(domain, ".k8s.io") ||
		slices.Contains(out, domain) {
		return out
	}
	return append(out, domain)
}

// textAttr is a text attribute of a present entity; empty otherwise.
func textAttr(model inventory.Reader, id inventory.EntityID,
	name string,
) string {
	a, ok := model.Attribute(id, name)
	if !ok {
		return ""
	}
	return a.Value.AsText()
}

// splitMarks splits a comma separated list, leaving out empty parts.
func splitMarks(list string) []string {
	var out []string
	for _, part := range strings.Split(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
