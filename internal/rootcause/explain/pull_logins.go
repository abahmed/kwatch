package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxLoginsShown bounds the registry logins a cause lists.
const maxLoginsShown = 3

// pullLogins says which registry logins the pods behind a refusing
// registry pull with, and what the registry answered. It reads only the
// pods' secret names and each Secret's type and change time. It is
// empty unless the cause is a registry refusing logins.
func (s Snapshot) pullLogins(c Cause) ([]rootcause.PullLogin, string) {
	if c.Root.Kind != kube.KindRegistry || c.Mode != pullAuth {
		return nil, ""
	}
	v := newView(s)
	found := map[[2]string]bool{}
	var refused string
	for _, id := range c.Covers {
		pod, ok := v.podOf(id)
		if !ok {
			continue
		}
		if refused == "" {
			refused = v.refusalOf(id)
		}
		for _, key := range s.secretKeys(pod) {
			found[key] = true
		}
	}
	return s.loginsOf(found), refused
}

// loginsOf describes each (namespace, secret) pair, sorted, up to
// maxLoginsShown.
func (s Snapshot) loginsOf(found map[[2]string]bool) []rootcause.PullLogin {
	keys := make([][2]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	var out []rootcause.PullLogin
	for _, key := range keys {
		if len(out) == maxLoginsShown {
			break
		}
		out = append(out, s.loginOf(key[0], key[1]))
	}
	return out
}

// loginOf looks the Secret up. Its values are never read: the model
// holds only digests, and only the type and change time are used.
func (s Snapshot) loginOf(namespace, name string) rootcause.PullLogin {
	login := rootcause.PullLogin{Namespace: namespace, Secret: name}
	if name == "" {
		login.State = rootcause.LoginNone
		return login
	}
	secret, ok := s.Model.Entity(inventory.CoreID(
		kube.KindSecret, namespace, name))
	switch {
	case ok:
		login.State = rootcause.LoginFound
		login.Type = attrText(secret, kube.AttrSecretType)
		login.Changed, _ = attrTime(secret, kube.AttrChanged)
	case s.synced(kube.KindSecret):
		login.State = rootcause.LoginMissing
	default:
		login.State = rootcause.LoginUnwatched
	}
	return login
}

// pullSecretsMissing reports an authentication refusal that is the
// pod's own doing: every image pull Secret it names is absent, so the
// kubelet pulled anonymously. The missing Secret is the cause then, and
// the registry, which refuses a pull with no login, is not at fault.
func (v *view) pullSecretsMissing(
	effect inventory.EntityID, class detection.Mode,
) bool {
	pod, ok := v.podOf(effect)
	if !ok || class != pullAuth || !v.s.synced(kube.KindSecret) {
		return false
	}
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	names := kube.PullSecretNames(attrText(entity, kube.AttrPullSecrets))
	for _, name := range names {
		if _, ok := v.s.Model.Entity(
			inventory.CoreID(kube.KindSecret, pod.Namespace, name)); ok {
			return false
		}
	}
	return len(names) > 0
}

// refusalOf is the registry refusal in the recent failed-pull notes of
// a container or pod, and of the containers of a pod.
func (v *view) refusalOf(id inventory.EntityID) string {
	sources := []inventory.EntityID{id}
	if id.Kind == kube.KindPod {
		sources = append(sources, v.s.Model.Related(id, inventory.PartOf,
			inventory.Incoming)...)
	} else if pod, ok := v.podOf(id); ok {
		sources = append(sources, pod)
	}
	for _, source := range sources {
		for _, note := range v.s.Model.Notes(source, v.since) {
			if note.Refusal != "" {
				return note.Refusal
			}
		}
	}
	return ""
}

// registryClass is the way a registry refuses the pulls of effect, when
// it is the registry's doing.
func (v *view) registryClass(
	effect inventory.EntityID,
) (detection.Mode, bool) {
	class := classifyRegistryPull(v.text(effect))
	if class == pullImage || v.pullSecretsMissing(effect, class) {
		return "", false
	}
	return class, true
}

// secretKeys are the (namespace, secret) pairs a pod pulls with; one
// pair with no secret when it names none, whatever the namespace.
func (s Snapshot) secretKeys(pod inventory.EntityID) [][2]string {
	entity, ok := s.Model.Entity(pod)
	if !ok {
		return nil
	}
	names := kube.PullSecretNames(attrText(entity, kube.AttrPullSecrets))
	if len(names) == 0 {
		return [][2]string{{"", ""}}
	}
	keys := make([][2]string, 0, len(names))
	for _, name := range names {
		keys = append(keys, [2]string{pod.Namespace, name})
	}
	return keys
}
