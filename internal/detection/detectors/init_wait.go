package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Init containers that run long are stuck only when they run far longer
// than this workload's own earlier init runs, or, with no history, past
// DefaultInitWait.
const (
	DefaultInitWait = 3 * time.Minute
	// initFloor is the least time an init container may run before it is
	// judged against history, so a quick wobble is never stuck.
	initFloor = time.Minute
)

// InitWait detects a pod held in Init by an init container that keeps
// running. An init container that fails and restarts is the Container
// detector's InitContainerError; this one is the container that is up
// and not finishing, often waiting for a dependency.
type InitWait struct{}

// Name implements detection.Detector.
func (InitWait) Name() string { return "init-wait" }

// Kinds implements detection.Detector.
func (InitWait) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindContainer}
}

// Detect implements detection.Detector.
func (InitWait) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	started := timestamp(e, kube.AttrStartedAt)
	if !flag(e, kube.AttrInit) || text(e, kube.AttrState) != "running" ||
		started.IsZero() || ctx.Model == nil {
		return nil
	}
	waited := ctx.Now.Sub(started)
	slow, quote := initRunsLong(ctx, e, waited)
	if !slow {
		// Whether it is stuck can change with the clock alone.
		ctx.RecheckAfter(time.Minute)
		return nil
	}
	finding := detection.Finding{
		Reason: reasons.InitContainerWaiting, Severity: detection.Warning,
		Since:   started,
		Summary: initWaitSummary(ctx, e, waited.Truncate(time.Minute)),
	}
	quote.quote(&finding)
	return []detection.Finding{finding}
}

// initRunsLong judges how long the init container has run against the
// workload's earlier init runs, or against DefaultInitWait without them.
func initRunsLong(
	ctx detection.Context, e inventory.Entity, waited time.Duration,
) (bool, judgement) {
	if waited < initFloor {
		return false, judgement{}
	}
	b := baselinesOf(ctx)
	workload, ok := workloadOfContainer(ctx, e)
	if b != nil && ok {
		cmp := b.Compare(workload, inventory.MetricInitSeconds,
			waited.Seconds())
		if cmp.Known {
			return cmp.Unusual, judge(cmp, "init running "+
				format.Duration(waited.Truncate(time.Minute))+
				" vs a usual "+usualDuration(cmp.Typical))
		}
	}
	return waited >= DefaultInitWait, judgement{}
}

// initWaitSummary says what is stuck and, when the container's own
// configuration names a Service, what it waits for and how that is.
func initWaitSummary(
	ctx detection.Context, e inventory.Entity, waited time.Duration,
) string {
	_, name, _ := cutName(e.ID.Name)
	lead := "is stuck in init: container " + name + " has been "
	for _, ref := range kube.ServiceCalls(e) {
		state, known := serviceState(ctx, ref.Service)
		if !known {
			continue
		}
		return lead + "waiting " + format.Duration(waited) + " for " +
			refName(ref) + " (" + state + ")"
	}
	return lead + "running for " + format.Duration(waited)
}

// refName writes a Service reference as "db:5432", or "db" without port.
func refName(ref kube.ServiceRef) string {
	if ref.Port == 0 {
		return ref.Service.Name
	}
	return ref.Service.Name + ":" + strconv.Itoa(ref.Port)
}

// serviceState words what the model knows of a Service an init container
// waits for. It is false when the Service exists and nothing is known to
// be wrong with it.
func serviceState(
	ctx detection.Context, id inventory.EntityID,
) (string, bool) {
	name := "Service " + id.Name + " in " + id.Namespace
	service, exists := ctx.Model.Entity(id)
	if !exists {
		if ctx.Synced(kube.KindService) {
			return name + " does not exist", true
		}
		return "", false
	}
	slices := ctx.Model.Related(id, inventory.Backs, inventory.Incoming)
	if len(slices) == 0 || text(service, kube.AttrSelector) == "" {
		return "", false
	}
	if _, ready, _ := sumEndpoints(ctx, slices); ready == 0 {
		return name + " has no ready endpoints", true
	}
	return "", false
}

// cutName splits the "pod/container" name of a container entity.
func cutName(name string) (pod, container string, ok bool) {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[:i], name[i+1:], true
		}
	}
	return "", name, false
}
