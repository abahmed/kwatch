package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type fakeLinkSink struct{ got []health.LinkStatus }

func (f *fakeLinkSink) SetDashboardLinks(l health.LinkStatus) {
	f.got = append(f.got, l)
}

func TestDashboardAddressLookupSaysWhyThereIsNone(t *testing.T) {
	const dash = " kwatch-dashboard "
	cases := map[string]struct {
		lines []string
		want  string
	}{
		"nothing": {nil, health.LinksNoRoute},
		"other": {[]string{"https k.example.com / grafana 80"},
			health.LinksNoRoute},
		"bad port": {[]string{"https k.example.com /" + dash + "9999"},
			health.LinksPortMismatch},
		"wildcard": {[]string{"https *.example.com /" + dash + "8061"},
			health.LinksUnusableAddress},
		"regex": {[]string{
			"https k.example.com /kw(/|$)(.*)" + dash + "dashboard",
			"https k.example.com /" + dash + "9999",
		}, health.LinksUnusableAddress},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			if tc.lines != nil {
				observeAddresses(t, model, ingressID("dash"), tc.lines...)
			}
			address, reason := testDashboard.lookup(model, nil)
			assert.Empty(t, address)
			assert.Equal(t, tc.want, reason)
		})
	}
}

// The link state is published and logged once per change, with the
// reason links are off.
func TestDashboardLinksReportsEachChangeOnce(t *testing.T) {
	out := captureKlog(t)
	model := inventory.NewModel(inventory.Options{})
	sink := &fakeLinkSink{}
	links := &dashboardLinks{target: testDashboard, model: model,
		sink: sink}

	require.Empty(t, links.address())
	require.Empty(t, links.address())
	observeAddresses(t, model, ingressID("dash"),
		"https k.example.com / kwatch-dashboard dashboard")
	require.Equal(t, "https://k.example.com", links.address())
	klog.Flush()

	require.Equal(t, []health.LinkStatus{
		{Reason: health.LinksNoRoute},
		{On: true, Address: "https://k.example.com"},
	}, sink.got)
	assert.Contains(t, out.String(), `reason="no_route"`)
	assert.Contains(t, out.String(), "dashboard links on")
}

func TestDashboardLinksNamesForbiddenRoutes(t *testing.T) {
	links := &dashboardLinks{target: testDashboard,
		model:     inventory.NewModel(inventory.Options{}),
		forbidden: func() bool { return true }}

	got := links.Status()

	assert.False(t, got.On)
	assert.Equal(t, health.LinksRoutesForbidden, got.Reason)
}

func TestRoutesForbiddenReadsTypedAndDiscoveredKinds(t *testing.T) {
	denied := &fakeAvailability{statuses: []kube.SourceStatus{{
		Resource: "ingresses", Group: "networking.k8s.io",
		Reason: kube.ReasonPermissionDenied}}}
	assert.True(t, routesForbidden(denied, nil))

	route := fakeDynamic{kube.DynamicStatus{Kinds: []kube.DynamicKind{{
		Resource: "httproutes.gateway.networking.k8s.io",
		Reason:   kube.ReasonPermissionDenied}}}}
	assert.True(t, routesForbidden(&fakeAvailability{}, route))

	other := fakeDynamic{kube.DynamicStatus{Kinds: []kube.DynamicKind{{
		Resource: "httproutes.gateway.networking.k8s.io",
		Reason:   kube.ReasonWatchBudget}}}}
	assert.False(t, routesForbidden(&fakeAvailability{}, other))
}

type fakePodLogs struct{ forbidden bool }

func (f fakePodLogs) PodLogsForbidden() bool { return f.forbidden }

// Refused pod logs degrade /health only.
func TestReportPodLogsIsAHealthSignal(t *testing.T) {
	sink := &fakeStatusSink{}

	reportPodLogs(sink, fakePodLogs{forbidden: true})
	require.Equal(t,
		recordedStatus{"degraded", "optional_permission_denied", false},
		sink.statuses[health.PodLogsComponent])

	reportPodLogs(sink, fakePodLogs{})
	require.Equal(t, recordedStatus{"running", "", true},
		sink.statuses[health.PodLogsComponent])
}

func TestCrashLogsRunOnlyWithInvestigationLogs(t *testing.T) {
	cases := map[string]struct {
		entitled, logs, want bool
	}{
		"pro with logs":    {true, true, true},
		"pro without logs": {true, false, false},
		"community":        {false, true, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			deps := investigationDeps(tc.entitled, tc.logs)
			assert.Equal(t, tc.want, crashLogsEnabled(deps))
		})
	}
}

func investigationDeps(entitled, logs bool) *serverDeps {
	cfg := &config.Config{}
	cfg.Investigation.Logs = logs
	deps := &serverDeps{runtime: config.CompileRuntimeConfig(cfg)}
	if entitled {
		deps.entitlements = feature.ProEntitlements()
	}
	return deps
}

// Until the routes are listed a missing address is not known to be
// missing: it is not reported as off, and the first message waits for
// the routes (bounded) so it carries the link found once they are.
func TestDashboardLinksWaitForTheRoutes(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	sink := &fakeLinkSink{}
	listed := make(chan struct{})
	links := &dashboardLinks{target: testDashboard, model: model,
		sink: sink, wait: time.Minute,
		settled: func() bool {
			select {
			case <-listed:
				return true
			default:
				return false
			}
		}}
	assert.False(t, links.Status().On)
	assert.Empty(t, sink.got, "off was reported before the routes")

	go func() {
		observeAddresses(t, model, ingressID("dash"),
			"https kwatch.example.com / kwatch-dashboard 8061")
		close(listed)
	}()
	assert.Equal(t, "https://kwatch.example.com", links.address())
	require.Len(t, sink.got, 1)
	assert.True(t, sink.got[0].On)
}

// Routes that never list hold the first message for the bound only.
func TestDashboardLinksWaitIsBounded(t *testing.T) {
	sink := &fakeLinkSink{}
	links := &dashboardLinks{target: testDashboard,
		model: inventory.NewModel(inventory.Options{}), sink: sink,
		wait: 10 * time.Millisecond, settled: func() bool { return false }}
	assert.Empty(t, links.address())
	require.Len(t, sink.got, 1)
	assert.Equal(t, health.LinksNoRoute, sink.got[0].Reason)
}

// routeKinds is a source whose kinds are all verifiable, with some
// turned off by configuration.
type routeKinds struct{ off []string }

func (r routeKinds) Disabled() []kube.SourceStatus {
	var out []kube.SourceStatus
	for _, resource := range r.off {
		out = append(out, kube.SourceStatus{Resource: resource})
	}
	return out
}

func (routeKinds) Verifiable(inventory.Kind) bool { return true }

// HTTPRoutes are discovered: before the first discovery pass the
// source knows nothing of them and calls them verifiable, so the wait
// must not end then, unless they are not watched by configuration.
func TestRoutesWaitForTheFirstDiscovery(t *testing.T) {
	assert.False(t, routesListed(routeKinds{}, false))
	assert.True(t, routesListed(routeKinds{}, true))
	assert.True(t, routesListed(routeKinds{off: []string{"httproutes"}},
		false))
}
