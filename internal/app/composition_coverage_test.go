package app

import (
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/incident"
)

func TestCompositionBuildsMonitorFamiliesAndOptionalBranches(t *testing.T) {
	engine := incident.NewEngineWithClock(
		incident.Config{}, clock.RealClock{},
	)
	deliveryManager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}},
	)
	clientset := fake.NewSimpleClientset()
	runtime := config.RuntimeConfigFor(&config.Config{})
	components := composeMonitorComponents(
		runtime, clientset, client.ClientSet{}, engine,
		deliveryManager, time.Now, nil,
	)
	if components.components.Pod.Processor == nil ||
		components.components.Workload.SourceConfig == nil {
		t.Fatal("monitor families were not composed")
	}
	if components.tlsProcessor != nil || components.controlPlane != nil {
		t.Fatal("disabled optional monitors were composed")
	}
	if tls, tlsConfig := composeTLSRuntime(
		runtime, engine, time.Now,
	); tls != nil ||
		tlsConfig != nil {
		t.Fatal("disabled TLS monitor returned a runtime")
	}
	if newNetworkGraphRun(
		runtime, nil, nil, nil, nil, nil,
	) != nil {
		t.Fatal("disabled network graph returned a runner")
	}
	if newStorageGraphRun(
		runtime, nil, nil, nil, nil, nil, nil,
	) != nil {
		t.Fatal("disabled storage graph returned a runner")
	}
}

func TestCompositionEnablesTLSAndIntegrationStatus(t *testing.T) {
	cfg := &config.Config{TlsMonitor: config.TlsMonitor{Enabled: true}}
	runtime := config.RuntimeConfigFor(cfg)
	engine := incident.NewEngineWithClock(
		incident.Config{}, clock.RealClock{},
	)
	tls, tlsConfig := composeTLSRuntime(runtime, engine, time.Now)
	if tls == nil || tlsConfig == nil {
		t.Fatal("enabled TLS monitor did not return both contracts")
	}
	integration, status := composeIntegrationRuntime(nil, tls, tlsConfig, nil)
	if integration.TLS == nil || status != nil {
		t.Fatal("integration runtime changed nil control-plane semantics")
	}
}
