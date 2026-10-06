package kube

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrSkipProbe is set on a Service annotated kwatch.io/skip-probe: "true".
// Automatic probing leaves such a Service alone.
const AttrSkipProbe = "probe.skip"

// serviceTarget is one Service port for automatic probing.
type serviceTarget struct {
	service inventory.EntityID
	address string
}

// serviceTargets lists the first TCP port of each Service that has a
// cluster IP, from the model. The list is bounded by autoProbeLimit, in
// name order so the same Services are probed every round, so a large
// cluster cannot turn probing into a scan; the first time Services are
// left out because of the cap, that is logged.
func (p *ActiveProber) serviceTargets() []serviceTarget {
	ids := p.cfg.Model.Entities(KindService)
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	var out []serviceTarget
	for _, id := range ids {
		port, ok := p.probeablePort(id)
		if !ok {
			continue
		}
		if len(out) >= autoProbeLimit {
			if !p.capLogged {
				p.capLogged = true
				klog.InfoS("automatic Service probing is capped",
					"component", "active-probe", "limit", autoProbeLimit)
			}
			break
		}
		out = append(out, serviceTarget{service: id,
			address: net.JoinHostPort(id.Name+"."+id.Namespace+".svc", port)})
	}
	return out
}

// probeablePort is the TCP port to probe on a Service, if it should be
// probed at all: not in an excluded namespace, not annotated to be
// skipped, not an ExternalName Service (that would dial an outside name
// from kwatch's pod), not a headless Service and with a TCP port.
func (p *ActiveProber) probeablePort(id inventory.EntityID) (string, bool) {
	if p.cfg.Excluded[id.Namespace] {
		return "", false
	}
	entity, ok := p.cfg.Model.Entity(id)
	if !ok {
		return "", false
	}
	if skip, ok := entity.Attribute(AttrSkipProbe); ok {
		if yes, _ := skip.Value.AsBool(); yes {
			return "", false
		}
	}
	if kind, ok := entity.Attribute(AttrServiceType); ok &&
		kind.Value.AsText() == "ExternalName" {
		return "", false
	}
	// A headless Service (ClusterIP None) has no virtual address to dial:
	// its name resolves to pod IPs, which are probed as the pods they are.
	if headless, ok := entity.Attribute(AttrHeadless); ok {
		if yes, _ := headless.Value.AsBool(); yes {
			return "", false
		}
	}
	attr, ok := entity.Attribute(AttrPorts)
	if !ok {
		return "", false
	}
	port := firstTCPPort(attr.Value.AsText())
	return port, port != ""
}

func (p *ActiveProber) checkService(
	ctx context.Context, target serviceTarget,
) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	err := p.dial(probeCtx, target.address)
	observation := probeObservation(target.service, p.cfg.Now(),
		p.cfg.Now().Sub(start), err)
	observation.Source = serviceProbeSource
	p.stampLimits(&observation)
	return observation
}

// firstTCPPort reads the first TCP port number from the
// "port/proto->target" list a ServiceSchema records. A UDP or SCTP port
// cannot be probed with a TCP connection and would look down forever.
func firstTCPPort(ports string) string {
	for _, entry := range strings.Split(ports, ",") {
		spec, _, _ := strings.Cut(entry, "->")
		port, proto, _ := strings.Cut(spec, "/")
		if _, err := strconv.Atoi(port); err != nil {
			continue
		}
		if proto == "" || proto == "TCP" {
			return port
		}
	}
	return ""
}
