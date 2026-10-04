package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/scope"
)

func TestScopedServicesHidesServicesOutsideNamespaceScope(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range []inventory.EntityID{
		inventory.CoreID(kube.KindService, "apps", "web"),
		inventory.CoreID(kube.KindService, "other", "api"),
		inventory.CoreID(kube.KindService, "kube-system", "dns"),
		inventory.CoreID(kube.KindNode, "", "node-a"),
	} {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: at, Entity: id,
		})
		require.NoError(t, err)
	}
	cfg := config.Config{
		AllowedNamespaces:   []string{"apps", "kube-system"},
		ForbiddenNamespaces: []string{"kube-system"},
	}
	findingScope, err := scope.New(config.RuntimeConfigFor(&cfg).Scope(),
		nil, false, nil)
	require.NoError(t, err)

	scoped := scopedServices{Reader: model, scope: findingScope}

	require.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindService, "apps", "web"),
	}, scoped.Entities(kube.KindService))
	require.Len(t, scoped.Entities(kube.KindNode), 1,
		"only Services are filtered")
}

func TestScopedServicesWithoutScopeKeepsEveryService(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Entity: inventory.CoreID(kube.KindService, "any", "web"),
	})
	require.NoError(t, err)

	scoped := scopedServices{Reader: model}

	require.Len(t, scoped.Entities(kube.KindService), 1)
}

func TestSourceConfigsShareDynamicStateAndMaintenance(t *testing.T) {
	deps := componentDeps()
	maintenance := config.MaintenanceConfig{
		Enabled: true, Annotation: "kwatch/hold",
		UntilAnnotation: "kwatch/hold-until",
	}
	dynamicCfg := dynamicSourceConfig(deps, nil, maintenance,
		inventory.NewModel(inventory.Options{}))
	dynamic := kube.NewDynamicSource(dynamicCfg)

	typed := typedSourceConfig(deps, nil, maintenance, dynamic,
		[]byte("key"))

	require.Same(t, dynamic, typed.Dynamic,
		"the typed source must ask the dynamic source about its kinds")
	want := kube.MaintenanceAnnotations{
		On: "kwatch/hold", Until: "kwatch/hold-until",
	}
	require.Equal(t, want, typed.Maintenance)
	require.Equal(t, want, dynamicCfg.Maintenance)
	require.Equal(t, []byte("key"), typed.DigestKey)
}
