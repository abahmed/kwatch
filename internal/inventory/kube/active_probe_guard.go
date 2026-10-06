package kube

import (
	"context"
	"errors"
	"net"
	"syscall"

	"github.com/abahmed/kwatch/internal/inventory"
)

// endpointGraceRounds is how many probe rounds an external endpoint may
// have no caller before its entity is retired. The grace keeps a pod
// restart, which briefly empties the model, from retiring endpoints.
const endpointGraceRounds = 3

// errBlockedAddress is the error of a dependency probe that resolved to
// an address kwatch refuses to dial.
var errBlockedAddress = errors.New("blocked: private address")

// blockedNetworks are address ranges with no public host: "this network"
// (0.0.0.0/8), carrier-grade NAT (RFC 6598), IETF protocol assignments,
// benchmarking (RFC 2544), reserved space (which holds the broadcast
// address) and the NAT64 prefix, which maps IPv4 addresses, private ones
// included, into IPv6.
var blockedNetworks = parseNetworks(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15",
	"240.0.0.0/4", "64:ff9b::/96",
)

func parseNetworks(cidrs ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("kwatch: bad blocked network " + cidr)
		}
		out = append(out, network)
	}
	return out
}

// privateAddress reports an address a dependency probe must not dial:
// loopback, link-local (including cloud metadata), private (RFC 1918 and
// unique local), unspecified and multicast addresses, and the ranges in
// blockedNetworks.
// External dependencies are on the public internet; a public name that
// resolves here is a way to make kwatch scan its own network.
func privateAddress(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, network := range blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// refusePrivate is a net.Dialer.Control: it runs once the name is
// resolved, for every address the dialer tries, so a name that resolves to
// a private address (or changes between lookups) cannot get past it.
func refusePrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedAddress
	}
	if privateAddress(net.ParseIP(host)) {
		return errBlockedAddress
	}
	return nil
}

// externalDialer opens TCP connections to public addresses only.
func externalDialer() func(
	context.Context, string, string,
) (net.Conn, error) {
	return (&net.Dialer{Control: refusePrivate}).DialContext
}

// retire counts, for every endpoint the prober has probed, the rounds
// without a caller, and returns Gone for those past the grace. called is
// every endpoint some pod in scope still calls, and targets those of them
// probed this round (the cap leaves some out): an endpoint that is still
// called but not probed is not retired.
func (p *ActiveProber) retire(
	targets []inventory.EntityID, called map[inventory.EntityID]bool,
) []inventory.Observation {
	if p.probed == nil {
		p.probed = map[inventory.EntityID]int{}
	}
	for _, id := range targets {
		p.probed[id] = 0
	}
	var gone []inventory.Observation
	for id := range p.probed {
		if called[id] {
			p.probed[id] = 0
			continue
		}
		p.probed[id]++
		if p.probed[id] >= endpointGraceRounds {
			delete(p.probed, id)
			gone = append(gone, inventory.Observation{
				Kind: inventory.Gone, Source: dependencyProbeSource,
				At: p.cfg.Now(), Entity: id,
			})
		}
	}
	return gone
}
