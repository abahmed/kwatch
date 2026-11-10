package explain

import (
	"regexp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pullFailing reports an effect that fails to pull an image.
func (v *view) pullFailing(effect inventory.EntityID) bool {
	return v.hasMode(effect, []detection.Mode{detection.ModeImagePull})
}

// isPullLogin reports whether the Secret is one the effect's pod pulls
// with and the registry refused the login: only then can an edit of the
// Secret, or its absence, be why the pull failed. A Secret the pod reads
// as an environment variable or a volume is not read by the kubelet when
// it pulls, and an image-level error (a wrong tag) is not about a login.
func (v *view) isPullLogin(secret, effect inventory.EntityID) bool {
	pod, ok := v.podOf(effect)
	if !ok || pod.Namespace != secret.Namespace {
		return false
	}
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	names := kube.PullSecretNames(attrText(entity, kube.AttrPullSecrets))
	if !slices.Contains(names, secret.Name) {
		return false
	}
	text, ok := v.pullText(effect)
	if !ok {
		return false
	}
	switch classifyPull(text) {
	case pullAuth:
		return true
	case pullImage:
		// Docker Hub answers a pull without credentials of a private
		// image as it answers a typo: "repository does not exist or may
		// require authorization". With the login the pod names absent,
		// the missing credentials are the likelier reading.
		return !v.s.Model.Exists(secret) &&
			mayRequireAuthPattern.MatchString(strings.ToLower(text))
	}
	return false
}

// mayRequireAuthPattern is a registry's refusal that is either an
// unknown image or one the client may not read without a login.
var mayRequireAuthPattern = regexp.MustCompile(
	`may require (authorization|'docker login')|requested access to ` +
		`the resource is denied`)

// pullOnly reports a Secret the effect's pod names only as an image
// pull Secret. The kubelet reads it when it pulls and at no other time,
// and pulls without it when it is absent: its absence or an edit of it
// cannot be why a container that has its image crashes or waits.
func (v *view) pullOnly(secret, effect inventory.EntityID) bool {
	pod, ok := v.podOf(effect)
	if !ok || pod.Namespace != secret.Namespace {
		return false
	}
	entity, ok := v.s.Model.Entity(pod)
	return ok && kube.OptionalReference(entity, secret) &&
		slices.Contains(kube.PullSecretNames(attrText(entity,
			kube.AttrPullSecrets)), secret.Name)
}

// withoutPseudo drops the pseudo modes (changes, absences), keeping the
// modes of the entity's own findings.
func withoutPseudo(modes []modeHealth) []modeHealth {
	var out []modeHealth
	for _, mh := range modes {
		if !mh.pseudo {
			out = append(out, mh)
		}
	}
	return out
}
