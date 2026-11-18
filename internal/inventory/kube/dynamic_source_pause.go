package kube

import (
	"context"
	"time"
)

// nextPass waits for the next discovery pass and reports false when ctx
// ended first. A CRD or APIService change starts one at once. The
// safety timer, which only catches what those watches missed and
// retries refused types, is optional work: while the API server is
// under pressure (DynamicConfig.Paused) its ticks are skipped, and the
// next tick after the pressure ends runs the pass.
func (d *DynamicSource) nextPass(
	ctx context.Context, tick <-chan time.Time,
) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-tick:
			if d.cfg.Paused != nil && d.cfg.Paused() {
				continue
			}
			d.retryRefused()
			return true
		case <-d.trigger:
			d.retryRefused()
			return true
		}
	}
}
