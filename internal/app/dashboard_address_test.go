package app

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var testDashboard = dashboardTarget{namespace: "kwatch",
	service: "kwatch-dashboard", port: 8061}

// observeAddresses puts an object with the given address lines
// ("scheme host path service port", space separated) in the model.
func observeAddresses(
	t *testing.T, model *inventory.Model, id inventory.EntityID,
	lines ...string,
) {
	t.Helper()
	tabbed := make([]string, len(lines))
	for i, line := range lines {
		tabbed[i] = strings.ReplaceAll(line, " ", "\t")
	}
	observe(t, model, id, kube.AttrPublicAddresses,
		strings.Join(tabbed, "\n"))
}

func observe(
	t *testing.T, model *inventory.Model, id inventory.EntityID,
	name, value string,
) {
	t.Helper()
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Entity: id,
		Attributes: map[string]inventory.Value{
			name: inventory.Text(value)},
	})
	require.NoError(t, err)
}

func ingressID(name string) inventory.EntityID {
	return inventory.CoreID(kube.KindIngress, "kwatch", name)
}

func TestDashboardAddressFromIngress(t *testing.T) {
	const dash = " kwatch-dashboard "
	cases := map[string]struct {
		line string
		want string
	}{
		"tls": {"https k.example.com /" + dash + "dashboard",
			"https://k.example.com"},
		"no tls": {"http k.example.com /" + dash + "8061",
			"http://k.example.com"},
		"path": {"https ops.example.com /kwatch/" + dash + "dashboard",
			"https://ops.example.com/kwatch"},
		"regex": {"https k.example.com /kw(/|$)(.*)" + dash + "dashboard",
			""},
		"other":    {"https k.example.com / grafana 80", ""},
		"bad port": {"https k.example.com /" + dash + "9999", ""},
		"wildcard": {"https *.example.com /" + dash + "8061", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			observeAddresses(t, model, ingressID("dash"), tc.line)
			assert.Equal(t, tc.want, testDashboard.address(model, nil))
		})
	}
}

func TestDashboardAddressPrefersTLSThenFirstHost(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	observeAddresses(t, model, ingressID("a"),
		"http a.example.com / kwatch-dashboard dashboard")
	observeAddresses(t, model, ingressID("b"),
		"https z.example.com / kwatch-dashboard dashboard",
		"https m.example.com / kwatch-dashboard dashboard")
	for range 5 {
		assert.Equal(t, "https://m.example.com",
			testDashboard.address(model, nil))
	}
}

func TestDashboardAddressNoneWithoutRoutes(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	assert.Empty(t, testDashboard.address(model, nil))
	other := inventory.CoreID(kube.KindIngress, "default", "dash")
	observeAddresses(t, model, other,
		"https k.example.com / kwatch-dashboard dashboard")
	assert.Empty(t, testDashboard.address(model, nil),
		"only kwatch's own namespace counts")
}

func TestDashboardAddressFromHTTPRoute(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	route := inventory.NewEntityID("gateway.networking.k8s.io",
		kube.KindHTTPRoute, "kwatch", "dash")
	observeAddresses(t, model, route,
		"https k.example.com / kwatch-dashboard 8061")
	assert.Equal(t, "https://k.example.com", testDashboard.address(model, nil))
}

// A Service port with its own number or name counts when it targets
// the dashboard port, as the installer's Services do.
func TestDashboardAddressFollowsServicePorts(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	observe(t, model,
		inventory.CoreID(kube.KindService, "kwatch", "kwatch-dashboard"),
		kube.AttrServicePortSpecs, "443:https:dashboard,80:web:8061")
	observeAddresses(t, model, ingressID("a"),
		"https a.example.com / kwatch-dashboard https")
	assert.Equal(t, "https://a.example.com", testDashboard.address(model, nil))
	observeAddresses(t, model, ingressID("a"),
		"https a.example.com / kwatch-dashboard 80")
	assert.Equal(t, "https://a.example.com", testDashboard.address(model, nil))
}

func TestDashboardTargetNamesServiceAfterDeployment(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ops")
	t.Setenv(dashboardDeploymentEnv, "watcher")
	assert.Equal(t, dashboardTarget{namespace: "ops",
		service: "watcher-dashboard", port: 9000},
		newDashboardTarget(9000))
}
