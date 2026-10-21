package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func licenseSecretChecks(checks []kube.Access) []kube.Access {
	var out []kube.Access
	for _, c := range checks {
		if c.Name == licenseInstanceSecret {
			out = append(out, c)
		}
	}
	return out
}

func checksFor(key string, watchSecrets bool) []kube.Access {
	cfg := config.DefaultConfig()
	cfg.License.Key = key
	cfg.Watch.Secrets = watchSecrets
	return permissionChecks(config.RuntimeConfigFor(cfg))
}

// The license Role is a grant of its own: with a key configured the audit
// checks it, so a missing Role shows up before the license stays pending.
func TestPermissionChecksIncludeTheLicenseSecretWhenAKeyIsSet(t *testing.T) {
	for _, watch := range []bool{true, false} {
		got := licenseSecretChecks(checksFor("KW1-x", watch))
		require.Len(t, got, 2, "watch.secrets=%v", watch)
		verbs := []string{got[0].Verb, got[1].Verb}
		require.ElementsMatch(t, []string{"get", "update"}, verbs)
		for _, c := range got {
			require.Equal(t, "secrets", c.Resource.Name)
			require.False(t, c.Required, "the license is optional")
		}
	}
}

func TestPermissionChecksOmitTheLicenseSecretWithoutAKey(t *testing.T) {
	require.Empty(t, licenseSecretChecks(checksFor("", true)))
}
