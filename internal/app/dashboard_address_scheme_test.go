package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// seenSchemes is a schemeSource with fixed answers.
type seenSchemes struct {
	byHost map[string]string
	https  bool
}

func (s seenSchemes) SchemeFor(host string) (string, bool) {
	scheme, ok := s.byHost[host]
	return scheme, ok
}

func (s seenSchemes) RequiresHTTPS() bool { return s.https }

// An HTTPRoute whose Gateway is not in the model has no known scheme:
// the scheme requests were seen with decides, then sign-in, then https.
func TestDashboardAddressOfRouteWithUnknownScheme(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	route := inventory.NewEntityID("gateway.networking.k8s.io",
		kube.KindHTTPRoute, "kwatch", "dash")
	observe(t, model, route, kube.AttrPublicAddresses,
		"\tk.example.com\t/\tkwatch-dashboard\t8061")
	cases := map[string]struct {
		seen schemeSource
		want string
	}{
		"nothing seen": {nil, "https://k.example.com"},
		"seen http": {seenSchemes{byHost: map[string]string{
			"k.example.com": "http"}}, "http://k.example.com"},
		"seen https": {seenSchemes{byHost: map[string]string{
			"k.example.com": "https"}}, "https://k.example.com"},
		"sign-in needs https": {seenSchemes{https: true,
			byHost: map[string]string{"k.example.com": "http"}},
			"https://k.example.com"},
		"other host seen": {seenSchemes{byHost: map[string]string{
			"x.example.com": "http"}}, "https://k.example.com"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, testDashboard.address(model, tc.seen))
		})
	}
}
