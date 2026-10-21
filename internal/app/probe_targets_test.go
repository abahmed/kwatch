package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
)

// A target the KwatchConfig added must reach the prober marked
// public-only; one the mounted configuration lists must not.
func TestProbeTargetsCarryTheOverlayMarkToTheProber(t *testing.T) {
	targets := probeTargets(config.ActiveProbeMonitor{
		HTTP: []config.HTTPProbeTarget{
			{Name: "m", URL: "https://m.example.com/"},
			{Name: "o", URL: "https://o.example.com/", FromOverlay: true}},
		TCP: []config.TCPProbeTarget{
			{Name: "m2", Address: "m.example.com:1"},
			{Name: "o2", Address: "o.example.com:1", FromOverlay: true}},
		DNS: []config.DNSProbeTarget{{Name: "d", Host: "d.example.com"}},
	})

	public := map[string]bool{}
	for _, target := range targets {
		public[target.Name] = target.PublicOnly
	}
	require.Equal(t, map[string]bool{"m": false, "o": true,
		"m2": false, "o2": true, "d": false}, public)
}
