package policy

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

// Decision is the result of one deterministic rule. Defer means this rule is
// not applicable yet; it is different from Suppress, which ends evaluation.
type Decision int

const (
	DecisionSuppress Decision = iota
	DecisionAlert
	DecisionDefer
)

type Detector interface {
	Detect(ctx *Context) Decision
}

// Context is the small mutable state passed through deterministic policy
// rules. It contains configuration and evaluation state only; Kubernetes
// clients, listers, events, and logs belong to the enrichment boundary.
type Context struct {
	Pod     *corev1.Pod
	EvType  string
	Runtime config.RuntimeConfig
	Now     func() time.Time

	Findings
	Container *ContainerContext
}

func (c *Context) runtime() config.RuntimeConfig {
	return c.Runtime
}

func (c *Context) allowedNamespaces() []string {
	return c.runtime().Scope().AllowedNamespaces()
}

func (c *Context) forbiddenNamespaces() []string {
	return c.runtime().Scope().ForbiddenNamespaces()
}

func (c *Context) allowedReasons() []string {
	return c.runtime().Scope().AllowedReasons()
}

func (c *Context) forbiddenReasons() []string {
	return c.runtime().Scope().ForbiddenReasons()
}

func (c *Context) suppressionIndex() config.SuppressionIndex {
	return c.runtime().Scope().SuppressionIndex()
}

func (c *Context) watchStartTime() time.Time {
	return c.runtime().Lifecycle().WatchStartTime()
}

func (c *Context) maintenance() config.MaintenanceConfig {
	return c.runtime().Monitors().Maintenance()
}

// Findings are the policy conclusions about the Pod.
type Findings struct {
	PodHasIssues        bool
	ContainersHasIssues bool
	PodReason           string
	PodMsg              string
	PodLastState        *model.ContainerState
}

// ContainerContext is retained as a package-local name for policy callers.
// The storage is shared with enrichment through the model package.
type ContainerContext = model.ContainerContext

func (c *Context) now() time.Time {
	return clock.RequireFunc(c.Now)()
}
