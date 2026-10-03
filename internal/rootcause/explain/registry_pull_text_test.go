package explain

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const unreachableRegistry = "registry.kwatch-e2e.invalid"

// registryPullFixture builds n Deployments pulling from one registry;
// text is what each container and pod say about the failed pull.
func registryPullFixture(
	f *fixture, n int, text func(image string) (note, message string),
) inventory.EntityID {
	var first inventory.EntityID
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("service-%d", i)
		image := unreachableRegistry + "/team/" + name + ":1"
		pod := f.workload("shop", name, 1)[0]
		f.relate(containerOf(pod), inventory.Pulls,
			inventory.CoreID(kube.KindImage, "", image))
		note, message := text(image)
		if note != "" {
			f.note(pod, note)
		}
		f.fail(containerOf(pod), "ImagePull", failingH, 2, message)
		if first.IsZero() {
			first = containerOf(pod)
		}
	}
	return first
}

// backoffOnly is what survives in a real cluster: the kubelet's
// repeated "Failed" events overwrite the detailed pull error, and the
// container only reports the back-off.
func backoffOnly(image string) (string, string) {
	return "Error: ImagePullBackOff",
		fmt.Sprintf("Back-off pulling image %q", image)
}

func detailedPull(image string) (string, string) {
	err := fmt.Sprintf("Failed to pull image %[1]q: failed to pull and "+
		"unpack image %[1]q: failed to resolve reference %[1]q: failed "+
		"to do request: Head \"https://%s/v2/team/manifests/1\": dial "+
		"tcp: lookup %s on 10.96.0.10:53: no such host", image,
		unreachableRegistry, unreachableRegistry)
	return err, fmt.Sprintf("Back-off pulling image %q", image)
}

func TestExplainRegistryPullTextBlamesSharedRegistry(t *testing.T) {
	cases := map[string]func(string) (string, string){
		"back-off message only":      backoffOnly,
		"full containerd pull error": detailedPull,
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			first := registryPullFixture(f, 4, text)
			requireCause(t, f.explain(), first,
				"registry//"+unreachableRegistry)
		})
	}
}

func TestExplainRegistryPullTextSparesSingleImage(t *testing.T) {
	f := newFixture(t)
	first := registryPullFixture(f, 1, backoffOnly)
	if c, ok := f.explain().CauseOf(first); ok &&
		c.Root.Kind == kube.KindRegistry {
		t.Fatalf("one failing image blamed registry %v", c.Root)
	}
}
