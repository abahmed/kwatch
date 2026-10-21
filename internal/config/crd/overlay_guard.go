package crd

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// mountedSettings is what the mounted configuration chose before a
// KwatchConfig was applied. The overlay may narrow it but not widen it.
type mountedSettings struct {
	telemetry       bool
	clusterCheckins bool
	updateCheckOff  bool
	probeHTTP       []config.HTTPProbeTarget
	probeTCP        []config.TCPProbeTarget
}

func snapshotMounted(cfg *config.Config) mountedSettings {
	return mountedSettings{
		telemetry:       cfg.Telemetry.Enabled,
		clusterCheckins: cfg.ClusterCheckins,
		updateCheckOff:  cfg.Upgrader.DisableUpdateCheck,
		probeHTTP:       slices.Clone(cfg.ActiveProbeMonitor.HTTP),
		probeTCP:        slices.Clone(cfg.ActiveProbeMonitor.TCP),
	}
}

// guard applies the rules that need the mounted values to compare with,
// after the overlay is decoded into cfg.
func (m mountedSettings) guard(cfg *config.Config) error {
	m.keepPrivacyChoices(cfg)
	return m.checkProbeTargets(cfg)
}

// keepPrivacyChoices lets a KwatchConfig turn telemetry, check-ins and the
// update check off but never back on: whoever mounted the configuration
// (or set the environment) made that choice for the installation.
func (m mountedSettings) keepPrivacyChoices(cfg *config.Config) {
	if !m.telemetry {
		cfg.Telemetry.Enabled = false
	}
	if !m.clusterCheckins {
		cfg.ClusterCheckins = false
	}
	if m.updateCheckOff {
		cfg.Upgrader.DisableUpdateCheck = true
	}
}

// checkProbeTargets refuses a probe target the overlay added that names
// an address kwatch itself would not dial for an external dependency:
// loopback, private, link-local (cloud metadata) and similar. Whoever can
// edit a KwatchConfig could otherwise aim kwatch's pod network position at
// internal services and read the result in an alert. Targets the mounted
// configuration lists are trusted. A DNS name passes here, since only a
// lookup says where it leads, so every target the overlay added is marked
// FromOverlay and the prober refuses, at connect time, a name that
// resolves to such an address.
func (m mountedSettings) checkProbeTargets(cfg *config.Config) error {
	probes := cfg.ActiveProbeMonitor
	for i, t := range probes.HTTP {
		if slices.Contains(m.probeHTTP, t) {
			continue
		}
		if u, err := url.Parse(t.URL); err == nil &&
			blockedProbeHost(u.Hostname()) {
			return blockedTarget("http", t.Name)
		}
		probes.HTTP[i].FromOverlay = true
	}
	for i, t := range probes.TCP {
		if slices.Contains(m.probeTCP, t) {
			continue
		}
		host, _, err := net.SplitHostPort(t.Address)
		if err == nil && blockedProbeHost(host) {
			return blockedTarget("tcp", t.Name)
		}
		probes.TCP[i].FromOverlay = true
	}
	return nil
}

func blockedTarget(kind, name string) error {
	return fmt.Errorf("KwatchConfig.spec.activeProbeMonitor.%s target "+
		"%q names a private or loopback address; list it in the "+
		"mounted configuration instead", kind, name)
}

// blockedProbeHost reports a literal IP address kube.PrivateProbeAddress
// refuses, or the name localhost.
func blockedProbeHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && kube.PrivateProbeAddress(ip)
}
