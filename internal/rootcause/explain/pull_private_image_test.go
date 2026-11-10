package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dockerHubRefusal is Docker Hub's answer to a pull of a private image
// without credentials, which it shares with a mistyped image name.
const dockerHubRefusal = "pull access denied for shop/api, repository " +
	"does not exist or may require authorization: server message: " +
	"insufficient_scope: authorization failed"

// privatePullFixture is n pods of shop/api, naming the pull Secrets
// secrets, refused by Docker Hub.
func privatePullFixture(
	f *fixture, n int, secrets string,
) inventory.EntityID {
	pods := f.workload("shop", "api", n)
	image := inventory.CoreID(kube.KindImage, "", "docker.io/shop/api:1")
	for _, pod := range pods {
		f.relate(containerOf(pod), inventory.Pulls, image)
		if secrets != "" {
			f.observe(pod, map[string]inventory.Value{
				kube.AttrPullSecrets: inventory.Text(secrets)})
			f.relate(pod, inventory.References,
				inventory.CoreID(kube.KindSecret, "shop", secrets))
		}
		f.noteRefused(pod, "Failed to pull image: "+dockerHubRefusal,
			dockerHubRefusal)
		f.fail(containerOf(pod), "ImagePull", failingH, 2,
			"Back-off pulling image docker.io/shop/api:1")
	}
	return containerOf(pods[0])
}

// A pod that names an absent pull Secret and is refused with Docker
// Hub's "may require authorization" pulls anonymously: the absent
// Secret is the cause, not the workload's image.
func TestAbsentPullSecretIsBlamedForAPrivateImageRefusal(t *testing.T) {
	f := newFixture(t)
	effect := privatePullFixture(f, 3, "regcred")
	requireCause(t, f.explain(), effect, "secret/shop/regcred")
}

// With no pull Secret named, the same answer stays an image error.
func TestPrivateImageRefusalWithoutAPullSecretBlamesNoSecret(
	t *testing.T,
) {
	f := newFixture(t)
	effect := privatePullFixture(f, 3, "")
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == kube.KindSecret {
		t.Fatalf("a Secret blamed: %v", c.Root)
	}
}
