package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The dashboard address is read from the Ingresses and HTTPRoutes, and
// those are optional kinds: they may finish their first list seconds
// after the sources kwatch needs to decide. A message written before
// that has no link, and the log says "links off: no_route" only to say
// "links on" a moment later. So until the routes are listed (or cannot
// be: forbidden), a missing address is not known to be missing: the
// first message waits for the routes, at most linkWait, once, and the
// state is not reported as off before then.

// linkWait bounds how long the first messages wait for the routes.
const linkWait = 10 * time.Second

// linkPoll is how often the wait looks at the routes again.
const linkPoll = 100 * time.Millisecond

// awaitRoutes waits, once, until the routes are listed or linkWait is
// over. Later calls return at once; callers racing the first one wait
// for it (sync.Once).
func (l *dashboardLinks) awaitRoutes() {
	l.waitOnce.Do(func() {
		defer l.waited.Store(true)
		if l.settled == nil || l.settled() {
			return
		}
		wait := l.wait
		if wait <= 0 {
			wait = linkWait
		}
		deadline := time.NewTimer(wait)
		defer deadline.Stop()
		tick := time.NewTicker(linkPoll)
		defer tick.Stop()
		for !l.settled() {
			select {
			case <-deadline.C:
				return
			case <-tick.C:
			}
		}
	})
}

// routesKnown reports whether a missing address is known to be
// missing: the routes are listed, cannot be, or the wait is over.
func (l *dashboardLinks) routesKnown() bool {
	return l.settled == nil || l.waited.Load() || l.settled()
}

// routesSettled reports that the Ingresses and HTTPRoutes the dashboard
// address comes from were listed, or that kwatch may not read them, or
// does not watch them by configuration.
func (p *pipelineBuild) routesSettled() bool {
	if p.source == nil || p.routesForbidden() {
		return true
	}
	return routesListed(p.source, p.dynamic == nil ||
		p.dynamic.Status().Ran)
}

// routeSource is the source as routesListed reads it.
type routeSource interface {
	Disabled() []kube.SourceStatus
	Verifiable(kind inventory.Kind) bool
}

// routesListed reports that each route kind is listed or turned off.
// HTTPRoute is a discovered kind: before the first discovery pass
// (discovered) the source knows nothing of it and calls it verifiable,
// so it counts as listed only after that pass.
func routesListed(source routeSource, discovered bool) bool {
	off := map[string]bool{}
	for _, s := range source.Disabled() {
		off[s.Resource] = true
	}
	return (off["ingresses"] || source.Verifiable(kube.KindIngress)) &&
		(off["httproutes"] ||
			discovered && source.Verifiable(kube.KindHTTPRoute))
}
