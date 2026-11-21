package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// blindToHiddenDNSConfig reports a cluster DNS whose servers keep
// crashing, the DNS server workload itself, or an external endpoint, blamed for failing lookups while the
// ConfigMaps the DNS servers read are unreadable to kwatch. Its Corefile may be the cause, so nothing is
// stated: the message says the cause is unknown and names what is
// unreadable. It records that on the failures it covers.
func (v *view) blindToHiddenDNSConfig(c *candidate) bool {
	if v.s.verifiable(kube.KindConfigMap) {
		return false
	}
	switch c.id.Kind {
	case kindClusterDNS:
		if !v.dnsServerCrashing() {
			return false
		}
	case KindExternalEndpoint:
		if !v.namedOnlyByLookups(c) {
			return false
		}
	default:
		// The DNS servers run and answer, but not for what clients
		// ask: the Corefile they serve from may be why, whichever
		// object is blamed for failures that are all lookups.
		if !v.dnsServersReady() || !v.onlyLookups(c) {
			return false
		}
	}
	note := rootcause.UnverifiedScope(
		inventory.CoreID(kube.KindConfigMap, "kube-system", ""))
	for _, effect := range c.direct() {
		v.unverified[effect] = rootcause.MergeUnverified(
			v.unverified[effect], []string{note})
	}
	return true
}

// onlyLookups reports a candidate whose every direct failure is a pod
// whose own error shows name resolution failing.
func (v *view) onlyLookups(c *candidate) bool {
	effects := c.direct()
	for _, effect := range effects {
		if !v.unitDNS(effect) {
			return false
		}
	}
	return len(effects) > 0
}

// namedOnlyByLookups reports an endpoint that the errors name, and only
// as what a failing lookup asked for.
func (v *view) namedOnlyByLookups(c *candidate) bool {
	names := v.namesOf(c.id)
	named := false
	for _, effect := range c.direct() {
		if !mentionsAny(v.errorText(effect), names) {
			continue
		}
		if !hasSignal(SignalDNS, v.errorText(effect)) {
			return false
		}
		named = true
	}
	return named
}

// dnsServerCrashing reports a DNS server that keeps crashing: it may be
// failing on the configuration it reads at start.
func (v *view) dnsServerCrashing() bool {
	for id, findings := range v.s.Findings {
		pod, ok := v.unitOf(id)
		if !ok || !v.dnsServer(pod) {
			continue
		}
		for _, f := range findings {
			if f.Mode.Within(detection.ModeCrashLoop) {
				return true
			}
		}
	}
	return false
}
