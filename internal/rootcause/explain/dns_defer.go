package explain

// deferClusterDNS lets the cause behind the cluster DNS servers' failure
// stand for the outage. The cluster DNS is the cause of the clients that
// cannot resolve, but only when nothing explains why its servers fail:
// when another viable candidate (a config map edit, a role, a memory
// limit, a workload flooding it) covers a DNS server, the outage follows
// from that candidate, which takes over the clients' coverage.
func (v *view) deferClusterDNS(viable []scored) []scored {
	var dns *scored
	for i := range viable {
		if viable[i].c.id.Kind == kindClusterDNS {
			dns = &viable[i]
		}
	}
	if dns == nil {
		return viable
	}
	for i := range viable {
		d := viable[i].c
		if d == dns.c || !v.coversDNSServer(d) {
			continue
		}
		for effect, how := range dns.c.covers {
			if _, covered := d.covers[effect]; !covered {
				d.setCover(effect, how)
			}
		}
		out := viable[:0:0]
		for _, s := range viable {
			if s.c != dns.c {
				out = append(out, s)
			}
		}
		return out
	}
	return viable
}

// coversDNSServer reports a candidate, other than the cluster DNS, that
// explains the failure of a cluster DNS server pod with a row of its
// own: a bare failure of the workload (the self row) says nothing more.
func (v *view) coversDNSServer(c *candidate) bool {
	if c.bestMatch().match.row.Name == selfRow.Name {
		// A server that fails by itself, with nothing behind it, is the
		// cluster DNS outage and no more.
		return false
	}
	for _, effect := range c.direct() {
		if pod, ok := v.podOf(effect); ok && v.dnsServer(pod) &&
			effect != c.id {
			return true
		}
	}
	return false
}
