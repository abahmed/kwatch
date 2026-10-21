package explain

import (
	"slices"

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
	return ok && classifyPull(text) == pullAuth
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
