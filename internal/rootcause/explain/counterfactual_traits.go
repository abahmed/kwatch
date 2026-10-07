package explain

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// shortDigestLen is how many hex digits of a digest a message shows.
const shortDigestLen = 12

// podTraits is what one pod can differ in from its replicas: the zone
// it runs in, the containers it carries and the build each runs.
type podTraits struct {
	zone string
	// images maps a container name to its image reference; init
	// containers are left out.
	images map[string]string
	// builds maps an image reference to the digest the kubelet says
	// it runs, and seen to when kwatch first saw that digest.
	builds map[string]string
	seen   map[string]time.Time
}

// traitsOf reads the traits of a pod.
func (v *view) traitsOf(pod inventory.EntityID) podTraits {
	t := podTraits{images: map[string]string{},
		builds: map[string]string{}, seen: map[string]time.Time{}}
	t.zone = v.zoneOf(pod)
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if e, ok := v.s.Model.Entity(id); ok && !attrBool(e, kube.AttrInit) {
			t.addContainer(id, e)
		}
	}
	return t
}

// zoneOf is the zone of the node a pod runs on, "" when unknown.
func (v *view) zoneOf(pod inventory.EntityID) string {
	node, ok := v.nodeOf(pod)
	if !ok {
		return ""
	}
	zone := ""
	for _, z := range v.s.Model.Related(node, inventory.PartOf,
		inventory.Outgoing) {
		if z.Kind == kube.KindZone && z.Name != "" {
			zone = z.Name
		}
	}
	return zone
}

// addContainer records the image of one container and, when the
// kubelet reports one, the digest it runs.
func (t *podTraits) addContainer(id inventory.EntityID, e inventory.Entity) {
	image := attrText(e, kube.AttrImage)
	if image == "" {
		return
	}
	t.images[id.Name[strings.LastIndex(id.Name, "/")+1:]] = image
	digest := shortDigest(attrText(e, kube.AttrImageID))
	if digest == "" || strings.Contains(image, "@") {
		return
	}
	t.builds[image] = digest
	if a, ok := e.Attribute(kube.AttrImageID); ok {
		t.seen[image] = a.Since
	}
}

// shortDigest is "sha256:" and the first hex digits of the digest in
// an image ID such as "registry/app@sha256:9f1c..."; empty when the ID
// holds no digest.
func shortDigest(imageID string) string {
	_, hex, found := strings.Cut(imageID, "sha256:")
	if !found || len(hex) < shortDigestLen {
		return ""
	}
	return "sha256:" + hex[:shortDigestLen]
}

// uniform returns the one value every pod has, or "" when the pods
// disagree or one has none.
func uniform(pods []podTraits, get func(podTraits) string) string {
	out := ""
	for i, p := range pods {
		value := get(p)
		if value == "" || (i > 0 && value != out) {
			return ""
		}
		out = value
	}
	return out
}

// containerSet is a pod's containers as "name image" lines, sorted.
func (t podTraits) containerSet() string {
	lines := make([]string, 0, len(t.images))
	for name, image := range t.images {
		lines = append(lines, name+" "+image)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// extraContainers lists the containers of t that other lacks.
func (t podTraits) extraContainers(other podTraits) []string {
	var out []string
	for name := range t.images {
		if _, ok := other.images[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
