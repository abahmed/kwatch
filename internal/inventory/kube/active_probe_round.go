package kube

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// KindKwatch is kwatch itself, a virtual entity that carries what kwatch
// learns about its own surroundings.
const KindKwatch inventory.Kind = "kwatch"

// KwatchSelf is the one entity of KindKwatch.
var KwatchSelf = inventory.CoreID(KindKwatch, "", "kwatch")

const (
	// AttrDependenciesUnreachable is how many probed dependencies failed
	// in a round where every one of them failed; zero once a round
	// again finds one that answers.
	AttrDependenciesUnreachable = "dependencies.unreachable.count"
	// minGuardedDependencies is how many dependencies a round needs
	// before "all of them failed" says anything about kwatch's own
	// network: one or two can really be down together.
	minGuardedDependencies = 3
)

// probeBatch runs probes concurrently and collects what they observe.
type probeBatch struct {
	mu           sync.Mutex
	wg           sync.WaitGroup
	observations []inventory.Observation
}

func (b *probeBatch) run(probe func() inventory.Observation) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		observation := probe()
		b.mu.Lock()
		b.observations = append(b.observations, observation)
		b.mu.Unlock()
	}()
}

func (b *probeBatch) wait() []inventory.Observation {
	b.wg.Wait()
	return b.observations
}

func (p *ActiveProber) round(ctx context.Context) {
	var checks, dependencies probeBatch
	for _, target := range p.cfg.Targets {
		checks.run(func() inventory.Observation {
			return p.check(ctx, target)
		})
	}
	if p.cfg.AutoServices {
		for _, target := range p.serviceTargets() {
			checks.run(func() inventory.Observation {
				return p.checkService(ctx, target)
			})
		}
	}
	var gone []inventory.Observation
	if p.cfg.AutoDependencies {
		targets, called := p.dependencyTargets()
		for _, endpoint := range targets {
			dependencies.run(func() inventory.Observation {
				return p.checkDependency(ctx, endpoint)
			})
		}
		gone = p.retire(targets, called)
	}
	observations := append(checks.wait(),
		p.guardDependencies(dependencies.wait())...)
	observations = append(observations, gone...)
	if len(observations) > 0 {
		p.cfg.Submit(ctx, observations...)
	}
}

// guardDependencies keeps a broken network of kwatch's own from
// looking like a broken dependency. When every dependency of a round
// failed, none is reported as down; one observation says that kwatch
// could not reach any of them. The next round that reaches one clears
// it. When pods that call those dependencies are failing too, the
// dependencies really are down, and each is reported as usual.
func (p *ActiveProber) guardDependencies(
	observations []inventory.Observation,
) []inventory.Observation {
	if len(observations) >= minGuardedDependencies &&
		allFailed(observations) && !p.usersFailing(observations) {
		p.restricted = true
		return []inventory.Observation{
			p.networkObservation(len(observations))}
	}
	if p.restricted {
		p.restricted = false
		observations = append(observations, p.networkObservation(0))
	}
	return observations
}

// usersFailing reports whether any pod that calls one of the probed
// dependencies looks broken: Running but not Ready, or with a container
// that has just restarted. A restricted kwatch network leaves those pods
// healthy; a real outage does not. Pods that are finished (Succeeded,
// Failed) or not started yet (Pending) are not Ready either, but say
// nothing about the dependency, so they do not count.
func (p *ActiveProber) usersFailing(
	observations []inventory.Observation,
) bool {
	for _, observation := range observations {
		users := p.cfg.Model.Related(observation.Entity, inventory.Calls,
			inventory.Incoming)
		for _, id := range users {
			if p.podLooksBroken(id) {
				return true
			}
		}
	}
	return false
}

func (p *ActiveProber) podLooksBroken(id inventory.EntityID) bool {
	pod, ok := p.cfg.Model.Entity(id)
	if !ok {
		return false
	}
	phase, _ := pod.Attribute(AttrPhase)
	switch corev1.PodPhase(phase.Value.AsText()) {
	case corev1.PodSucceeded, corev1.PodFailed, corev1.PodPending:
		return false
	}
	if ready, known := pod.Attribute(AttrReady); known {
		if up, _ := ready.Value.AsBool(); !up {
			return true
		}
	}
	for _, container := range p.cfg.Model.Related(id, inventory.PartOf,
		inventory.Incoming) {
		c, ok := p.cfg.Model.Entity(container)
		if !ok {
			continue
		}
		if p.restartingNow(c) {
			return true
		}
	}
	return false
}

// recentRestart is how recently a container must have terminated to
// count as failing now. The restart counter alone is for the life of the
// pod, and an old restart says nothing about a dependency today.
const recentRestart = 10 * time.Minute

// restartingNow reports a container that is waiting or terminated and
// whose last termination was recent.
func (p *ActiveProber) restartingNow(c inventory.Entity) bool {
	restarts, _ := c.Attribute(AttrRestarts)
	if count, _ := restarts.Value.AsNumber(); count == 0 {
		return false
	}
	state, _ := c.Attribute(AttrState)
	switch state.Value.AsText() {
	case "waiting", "terminated":
	default:
		return false
	}
	finished, ok := c.Attribute(AttrLastFinished)
	if !ok {
		return false
	}
	return p.cfg.Now().Sub(finished.Value.AsTime()) <= recentRestart
}

func allFailed(observations []inventory.Observation) bool {
	for _, observation := range observations {
		healthy, ok := observation.Attributes[AttrHealthy].AsBool()
		if !ok || healthy {
			return false
		}
	}
	return true
}

func (p *ActiveProber) networkObservation(
	failed int,
) inventory.Observation {
	observation := inventory.Observation{
		Kind: inventory.Observed, Source: dependencyProbeSource,
		At: p.cfg.Now(), Entity: KwatchSelf,
		Attributes: map[string]inventory.Value{
			AttrDependenciesUnreachable: inventory.Number(float64(failed)),
		},
	}
	p.stampLimits(&observation)
	return observation
}

// stampLimits records how long the probe waits and how long a failure
// must last before it is reported.
func (p *ActiveProber) stampLimits(observation *inventory.Observation) {
	observation.Attributes[AttrFailureDuration] = inventory.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	observation.Attributes[AttrProbeTimeoutSeconds] = inventory.Number(
		p.cfg.Timeout.Seconds())
}
