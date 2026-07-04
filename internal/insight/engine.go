package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

type Insight struct {
	Cause         string
	Impact        string
	Pattern       string
	AffectedCount int
	RecentChanges []context.Change
}

type Engine struct {
	graph   *context.ResourceGraph
	tracker *context.ChangeTracker
	now     func() time.Time
}

func NewEngine(graph *context.ResourceGraph, tracker *context.ChangeTracker) *Engine {
	return &Engine{
		graph:   graph,
		tracker: tracker,
		now:     time.Now,
	}
}

func (e *Engine) Analyze(inc *model.Incident) *Insight {
	ins := &Insight{}

	e.determineCause(inc, ins)
	e.describeImpact(inc, ins)
	e.checkRecentChanges(inc, ins)

	return ins
}

func (e *Engine) determineCause(inc *model.Incident, ins *Insight) {
	if e.graph == nil {
		return
	}
	ns := inc.Namespace
	name := inc.Name

	deps := e.graph.DependenciesOf(inc.Resource, ns, name)
	if len(deps) == 0 {
		return
	}

	nodeKey := "node//" + inc.NodeName
	if inc.NodeName != "" {
		for _, d := range deps {
			if d == nodeKey {
				ins.Cause = fmt.Sprintf("node %s may be unhealthy", inc.NodeName)
				ins.Pattern = "node_failure"
				return
			}
		}
	}

	if inc.Resource == "pod" && inc.OwnerKind != "" {
		ownerPrefix := strings.ToLower(inc.OwnerKind) + "/" + ns + "/"
		for _, d := range deps {
			if strings.HasPrefix(d, ownerPrefix) {
				ownerName := d[len(ownerPrefix):]
				ins.Cause = fmt.Sprintf("owning %s %s is unhealthy", inc.OwnerKind, ownerName)
				ins.Pattern = "rollout_failure"
				return
			}
		}
	}

	for _, d := range deps {
		if len(d) > 9 && d[:9] == "configmap" {
			ins.Cause = "referenced ConfigMap may have changed or is misconfigured"
			ins.Pattern = "config_error"
			return
		}
		if len(d) > 6 && d[:6] == "secret" {
			ins.Cause = "referenced Secret may have changed or is misconfigured"
			ins.Pattern = "config_error"
			return
		}
		if len(d) > 3 && d[:3] == "pvc" {
			ins.Cause = "referenced PVC may be unavailable"
			ins.Pattern = "config_error"
			return
		}
	}
}

func (e *Engine) describeImpact(inc *model.Incident, ins *Insight) {
	if e.graph == nil {
		return
	}
	switch inc.Resource {
	case "node":
		deps := e.graph.DependentsByType("node", "", inc.NodeName, "pod")
		if len(deps) > 0 {
			ins.Impact = fmt.Sprintf("%d pods on this node", len(deps))
		}
	case "pod":
		if inc.NodeName != "" {
			deps := e.graph.DependentsByType("pod", inc.Namespace, inc.Name, "service")
			if len(deps) > 0 {
				ins.Impact = fmt.Sprintf("affects %d service(s)", len(deps))
			}
		}
	case "deployment", "statefulset", "daemonset":
		allDeps := e.graph.DependentsOf(inc.Resource, inc.Namespace, inc.Name)
		if len(allDeps) > 0 {
			ins.Impact = fmt.Sprintf("%d dependent resource(s)", len(allDeps))
		}
	case "configmap":
		deps := e.graph.DependentsByType("configmap", inc.Namespace, inc.Name, "pod")
		if len(deps) > 0 {
			ins.Impact = fmt.Sprintf("%d pod(s) reference this configmap", len(deps))
		}
	case "secret":
		deps := e.graph.DependentsByType("secret", inc.Namespace, inc.Name, "pod")
		if len(deps) > 0 {
			ins.Impact = fmt.Sprintf("%d pod(s) reference this secret", len(deps))
		}
	case "pvc":
		deps := e.graph.DependentsByType("pvc", inc.Namespace, inc.Name, "pod")
		if len(deps) > 0 {
			ins.Impact = fmt.Sprintf("%d pod(s) use this pvc", len(deps))
		}
	}
}

func (e *Engine) checkRecentChanges(inc *model.Incident, ins *Insight) {
	if e.tracker == nil {
		return
	}
	recent := e.tracker.RecentChangesBefore(5 * time.Minute)
	filtered := make([]context.Change, 0, len(recent))
	for _, c := range recent {
		if c.Resource == inc.Resource && c.Namespace == inc.Namespace && c.Name == inc.Name {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) > 0 {
		ins.RecentChanges = filtered
		return
	}

	for _, c := range recent {
		if c.Namespace == inc.Namespace {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) > 3 {
		filtered = filtered[:3]
	}
	if len(filtered) > 0 {
		ins.RecentChanges = filtered
	}
}
