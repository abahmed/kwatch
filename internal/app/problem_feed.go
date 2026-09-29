package app

import (
	"sync"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/problem"
)

// problemFeed adapts the per-session core engine to the health server,
// which is configured once before the engine exists. The source is set when
// the core starts and cleared when it stops; without one the list is empty.
type problemFeed struct {
	mu     sync.RWMutex
	source func() []problem.Problem
}

func (f *problemFeed) set(source func() []problem.Problem) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.source = source
	f.mu.Unlock()
}

func (f *problemFeed) clear() { f.set(nil) }

// Snapshot returns a bounded, safe view of the live problems.
func (f *problemFeed) Snapshot() []health.ProblemView {
	if f == nil {
		return []health.ProblemView{}
	}
	f.mu.RLock()
	source := f.source
	f.mu.RUnlock()
	if source == nil {
		return []health.ProblemView{}
	}
	problems := source()
	if len(problems) > health.MaxProblemViews {
		problems = problems[:health.MaxProblemViews]
	}
	out := make([]health.ProblemView, 0, len(problems))
	for i := range problems {
		out = append(out, buildProblemView(&problems[i]))
	}
	return out
}

func buildProblemView(p *problem.Problem) health.ProblemView {
	view := health.ProblemView{
		ID:    p.ID,
		State: p.State.String(),
		Tier:  p.Tier.String(),
		Root: health.RootView{
			Kind:      string(p.Root.Kind),
			Namespace: p.Root.Namespace,
			Name:      p.Root.Name,
		},
		Opened:      p.Opened,
		Announced:   p.Announced,
		Members:     len(p.Members),
		ImpactCount: len(p.Impact),
	}
	if p.Cause != nil {
		view.Cause = p.Cause.Summary
		view.Confidence = p.Cause.Score
	}
	return view
}

// openHealth configures the diagnostics once and opens the listener; the
// supervisor then serves it.
func openHealth(deps *serverDeps) error {
	if err := deps.healthServer.ConfigureDependencies(
		health.Dependencies{Problems: deps.problems},
	); err != nil {
		return err
	}
	return deps.healthServer.Open()
}
