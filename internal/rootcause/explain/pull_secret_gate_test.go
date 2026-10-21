package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	pullAuthText = "Failed to pull image: failed to authorize: " +
		"401 Unauthorized"
	pullTypoText = "Failed to pull image \"registry.example.com/api:v2\": " +
		"manifest unknown: manifest unknown"
)

// pullSecretCase edits the Secret "creds" of shop three minutes before
// two pods fail to pull with text. asPullSecret says whether the pods
// name it in imagePullSecrets; otherwise they read it from the
// environment.
func pullSecretCase(
	f *fixture, asPullSecret bool, text string,
) (inventory.EntityID, inventory.EntityID) {
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "creds", pods...)
	f.change(secret, 1, "data.password")
	for _, pod := range pods {
		if asPullSecret {
			f.observe(pod, map[string]inventory.Value{
				kube.AttrPullSecrets: inventory.Text("creds")})
		}
		f.noteRefused(pod, text, "")
		f.fail(containerOf(pod), "ImagePull", failingH, 4, text)
	}
	return secret, containerOf(pods[0])
}

// TestSecretIsBlamedForAPullOnlyAsALogin: an edited Secret explains a
// failed pull when the pods pull with it and the registry refused the
// login; an environment Secret, or an image-level error, is not it.
func TestSecretIsBlamedForAPullOnlyAsALogin(t *testing.T) {
	for _, tc := range []struct {
		name         string
		asPullSecret bool
		text         string
		blamed       bool
	}{
		{"pull secret and refused login", true, pullAuthText, true},
		{"pull secret and a typo in the tag", true, pullTypoText, false},
		{"environment secret and a typo in the tag", false, pullTypoText,
			false},
		{"environment secret and refused login", false, pullAuthText,
			false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			secret, _ := pullSecretCase(f, tc.asPullSecret, tc.text)
			if got := blames(f.explain(), secret); got != tc.blamed {
				t.Fatalf("secret blamed = %v, want %v", got, tc.blamed)
			}
		})
	}
}
