package cloudmeta

import (
	"net"
	"strings"
)

// names are the host names of cloud metadata services.
var names = map[string]bool{
	"metadata.google.internal": true,
	"metadata.goog":            true,
}

// addresses are metadata addresses outside the link-local ranges:
// AWS over IPv6 and Alibaba Cloud.
var addresses = []net.IP{
	net.ParseIP("fd00:ec2::254"),
	net.ParseIP("100.100.100.200"),
}

// nat64 is the well-known NAT64 prefix, which can carry an IPv4 metadata
// address inside IPv6.
var nat64 = &net.IPNet{IP: net.ParseIP("64:ff9b::"),
	Mask: net.CIDRMask(96, 128)}

// Addresses returns the metadata addresses outside the link-local
// ranges, for tests that check a guard refuses each of them.
func Addresses() []net.IP {
	out := make([]net.IP, len(addresses))
	for i, ip := range addresses {
		out[i] = append(net.IP(nil), ip...)
	}
	return out
}

// Address reports a cloud metadata address: the link-local ranges
// (169.254.0.0/16 and fe80::/10, which hold 169.254.169.254), the other
// metadata addresses, and any of them inside the NAT64 prefix. A nil
// address is not one.
func Address(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if nat64.Contains(ip) {
		ip = net.IP(ip.To16()[12:16])
	}
	if ip.IsLinkLocalUnicast() {
		return true
	}
	for _, m := range addresses {
		if m.Equal(ip) {
			return true
		}
	}
	return false
}

// Host reports a URL host (a name or an address, with an optional zone)
// that names a metadata service.
func Host(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if names[host] {
		return true
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return Address(net.ParseIP(host))
}
