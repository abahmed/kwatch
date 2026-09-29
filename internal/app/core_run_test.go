package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
)

// restDiscovery gives the fake clientset a Discovery client whose REST
// client reaches a local server, as the control-plane prober requires.
type restDiscovery struct {
	*fake.Clientset
	discovery *discovery.DiscoveryClient
}

func (r restDiscovery) Discovery() discovery.DiscoveryInterfaces {
	return r.discovery
}

func coreDeps(t *testing.T, cfg *config.Config) *serverDeps {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}))
	t.Cleanup(api.Close)
	dc, err := discovery.NewDiscoveryClientForConfig(
		&rest.Config{Host: api.URL})
	require.NoError(t, err)
	clientset := restDiscovery{
		Clientset: fake.NewSimpleClientset(), discovery: dc,
	}
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	deps := &serverDeps{
		runtime: runtimeWith(cfg),
		clients: client.ClientSet{
			Kubernetes: clientset,
			Dynamic: dynamicfake.
				NewSimpleDynamicClientWithCustomListKinds(
					runtime.NewScheme(), dynamicListKinds()),
			Discovery: dc,
			Resolver:  staticResolver{},
			Clock:     clock.RealClock{},
		},
		healthServer: server,
		readiness:    newReadinessCoordinator(server),
		deliveryManager: delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}}),
		coreProgress: newComponentProgress(time.Now()),
	}
	return deps
}

func TestRunCoreBecomesReadyAndStopsOnCancel(t *testing.T) {
	deps := coreDeps(t, &config.Config{})
	deps.readiness.begin(1, false)
	deps.readiness.setCurrent("state", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)

	go func() { done <- runCore(ctx, deps, openTestStore(t)) }()

	require.Eventually(t, deps.healthServer.Ready, 10*time.Second,
		5*time.Millisecond, "core never reported ready")
	require.False(t, deps.coreProgress.LastProgress().IsZero())
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("runCore did not stop sources after cancellation")
	}
}

func TestRunCoreRunsActiveProberWhenConfigured(t *testing.T) {
	deps := coreDeps(t, &config.Config{
		ActiveProbeMonitor: config.ActiveProbeMonitor{Enabled: true},
	})
	deps.readiness.begin(1, false)
	deps.readiness.setCurrent("state", true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- runCore(ctx, deps, openTestStore(t)) }()
	require.Eventually(t, deps.healthServer.Ready, 10*time.Second,
		5*time.Millisecond)
	cancel()

	require.NoError(t, <-done)
}

func TestRunCoreFailsWhenStateStoreIsClosed(t *testing.T) {
	deps := coreDeps(t, &config.Config{})
	s := openTestStore(t)
	require.NoError(t, s.Close())

	err := runCore(context.Background(), deps, s)

	require.Error(t, err, "restoring from a closed store must fail")
}

type staticResolver struct{}

func (staticResolver) LookupHost(
	context.Context, string,
) ([]string, error) {
	return []string{"10.0.0.1"}, nil
}

func (staticResolver) LookupAddr(
	context.Context, string,
) ([]string, error) {
	return nil, nil
}

// dynamicListKinds registers every resource the dynamic source lists,
// which the fake dynamic client requires up front.
func dynamicListKinds() map[schema.GroupVersionResource]string {
	kinds := map[schema.GroupVersionResource]string{}
	for _, r := range []struct{ group, resource, kind string }{
		{"apiextensions.k8s.io", "customresourcedefinitions",
			"CustomResourceDefinitionList"},
		{"apiregistration.k8s.io", "apiservices", "APIServiceList"},
		{"certificates.k8s.io", "certificatesigningrequests",
			"CertificateSigningRequestList"},
		{"flowcontrol.apiserver.k8s.io", "flowschemas", "FlowSchemaList"},
		{"flowcontrol.apiserver.k8s.io", "prioritylevelconfigurations",
			"PriorityLevelConfigurationList"},
		{"admissionregistration.k8s.io", "validatingadmissionpolicies",
			"ValidatingAdmissionPolicyList"},
		{"admissionregistration.k8s.io", "validatingadmissionpolicybindings",
			"ValidatingAdmissionPolicyBindingList"},
		{"resource.k8s.io", "resourceclaims", "ResourceClaimList"},
	} {
		kinds[schema.GroupVersionResource{
			Group: r.group, Version: "v1", Resource: r.resource,
		}] = r.kind
	}
	return kinds
}
