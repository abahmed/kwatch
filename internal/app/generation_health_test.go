package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// statusOf is the HTTP status of path on the health port, or 0 when
// nothing listens.
func statusOf(port int, path string) int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// During an edition rebuild the kubelet's liveness probe must keep
// getting answers: the rebuild's steps can last as long as its tolerance.
// The old listener stays up, not ready, until the next generation has
// built everything slow and takes the port over.
func TestRebuildKeepsAnsweringLivenessUntilTheNextGenerationBinds(
	t *testing.T,
) {
	port := freeTCPPort(t)
	h := newGenHarness(t)
	h.health = func(int) config.HealthCheck {
		return config.HealthCheck{Enabled: true, Port: port}
	}
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return endGeneration(deps, restartTo(feature.ProEntitlements()))
		}
		return endGeneration(deps, nil)
	}
	var live, ready int
	h.bootErr = func(gen, _ int) error {
		if gen == 2 {
			live = statusOf(port, "/healthz")
			ready = statusOf(port, "/readyz")
		}
		return nil
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, http.StatusOK, live,
		"liveness answers while the next generation is built")
	require.Equal(t, http.StatusServiceUnavailable, ready,
		"readiness is withdrawn during the rebuild")
	require.Empty(t, h.sleeps.waits,
		"the port was free when the next generation bound it")
}
