package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/persistence"
)

func TestConfigurePersistenceDefersRestoreUntilActivation(t *testing.T) {
	client := fake.NewSimpleClientset()
	manager := persistence.NewManagerWithClock(
		client, "kwatch", clock.RealClock{},
	)
	runtime := config.RuntimeConfigFor(&config.Config{})
	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	readiness := newReadinessCoordinator(healthServer)

	setup := configurePersistence(
		manager, runtime, time.Now, healthServer, readiness,
	)
	require.NotNil(t, setup.tracker)
	require.NotNil(t, setup.feedbackStore)
	require.Empty(t, client.Actions())
}
