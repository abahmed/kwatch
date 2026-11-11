package dashboard

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"k8s.io/klog/v2"
)

// proxySet is the peers allowed to speak for a client: the proxies
// (CIDRs or single addresses) that terminate TLS in front of kwatch.
type proxySet []netip.Prefix

// parseProxies reads a list of CIDRs and bare addresses. Entries that
// do not parse are returned as errors and left out.
func parseProxies(list []string) (proxySet, []error) {
	var set proxySet
	var errs []error
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		if p, err := netip.ParsePrefix(entry); err == nil {
			warnBroadProxy(p)
			set = append(set, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"trusted proxy %q is not an address or CIDR", entry))
			continue
		}
		set = append(set, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return set, errs
}

// broadBits are the shortest prefixes trusted without a warning: a
// wider range usually holds every pod of the cluster, and any of them
// could then name any client and claim https.
const (
	broadBits4 = 16
	broadBits6 = 48
)

// warnBroadProxy logs when a trusted range is wider than broadBits.
func warnBroadProxy(p netip.Prefix) {
	limit := broadBits4
	if p.Addr().Is6() && !p.Addr().Is4In6() {
		limit = broadBits6
	}
	if p.Bits() >= limit {
		return
	}
	klog.InfoS("WARNING: dashboard.trustedProxies entry is wide: any "+
		"peer inside it can name any client and claim https; list "+
		"only the proxies in front of kwatch", "component",
		"dashboard-server", "entry", p.String())
}

// limitKey is the key sign-in failures of addr count under: the address
// itself, or its /64 for IPv6, which one host can rotate through freely.
func limitKey(addr string) string {
	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is6() || ip.Is4In6() {
		return addr
	}
	p, err := ip.Prefix(64)
	if err != nil {
		return addr
	}
	return p.String()
}

func (p proxySet) contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// peerTrusted reports whether the connecting peer is a trusted proxy.
func (a *Auth) peerTrusted(r *http.Request) bool {
	if len(a.trusted) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(remoteHost(r))
	return err == nil && a.trusted.contains(addr)
}

// isHTTPS reports whether the browser reached kwatch over https: kwatch
// served TLS itself, or a trusted proxy that terminated TLS said so in
// X-Forwarded-Proto. The header counts only from a trusted peer, so a
// port-forward or any other client can never claim https.
func (a *Auth) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !a.peerTrusted(r) {
		return false
	}
	return strings.EqualFold(lastHeaderValue(r, "X-Forwarded-Proto"),
		"https")
}

// forwardedClient is the client a trusted proxy named. X-Forwarded-For
// is walked from the right, skipping hops that are trusted proxies
// themselves; the first other hop is the client. Empty when the peer
// is not trusted or a hop is not an IP address (with or without a port,
// see hopAddr), so a made-up value falls back to the peer. A header
// from an untrusted peer is ignored and logged once, so a missing
// trustedProxies setting is noticed.
func (a *Auth) forwardedClient(r *http.Request) string {
	if !a.peerTrusted(r) {
		a.noteUntrustedForwarded(r)
		return ""
	}
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(line, ",")...)
	}
	client := ""
	for i := len(hops) - 1; i >= 0; i-- {
		addr, ok := hopAddr(hops[i])
		if !ok {
			return ""
		}
		client = addr.Unmap().String()
		if !a.trusted.contains(addr) {
			break
		}
	}
	return client
}

// hopAddr reads one X-Forwarded-For hop: a bare address, or an address
// with a port ("203.0.113.7:4711", "[2001:db8::7]:4711"), which some
// proxies append, or in brackets alone. Without this every client behind
// such a proxy would share the proxy's sign-in failure limit.
func hopAddr(hop string) (netip.Addr, bool) {
	hop = strings.TrimSpace(hop)
	if addr, err := netip.ParseAddr(hop); err == nil {
		return addr, true
	}
	if withPort, err := netip.ParseAddrPort(hop); err == nil {
		return withPort.Addr(), true
	}
	if bare, ok := strings.CutPrefix(hop, "["); ok {
		if bare, ok = strings.CutSuffix(bare, "]"); ok {
			if addr, err := netip.ParseAddr(bare); err == nil {
				return addr, true
			}
		}
	}
	return netip.Addr{}, false
}

// noteUntrustedForwarded logs once when a peer that is not a trusted
// proxy sends X-Forwarded-For: the dashboard sits behind a proxy that
// dashboard.trustedProxies does not name.
func (a *Auth) noteUntrustedForwarded(r *http.Request) {
	if r.Header.Get("X-Forwarded-For") == "" ||
		!a.warnedForwarded.CompareAndSwap(false, true) {
		return
	}
	klog.InfoS("WARNING: dashboard request carries X-Forwarded-For "+
		"from a peer that is not in dashboard.trustedProxies; the "+
		"header is ignored, so sign-in failures are limited per "+
		"connecting address and https is not detected",
		"component", "dashboard-server", "peer", remoteHost(r))
}

// lastHeaderValue is the last comma-separated value of a header that may
// repeat, trimmed.
func lastHeaderValue(r *http.Request, name string) string {
	values := r.Header.Values(name)
	if len(values) == 0 {
		return ""
	}
	parts := strings.Split(values[len(values)-1], ",")
	return strings.TrimSpace(parts[len(parts)-1])
}
