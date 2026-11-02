package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/scope"
)

// coverageSources are what /api/coverage reads; *kube.Source and
// *kube.DynamicSource provide them.
type coverageSources struct {
	typed interface {
		WatchedKinds() []kube.WatchedKind
		Unavailable() []kube.SourceStatus
		Disabled() []kube.SourceStatus
		SyncedAt() time.Time
	}
	dynamic dynamicCoverage
	model   inventory.Reader
	scope   *scope.Scope
	// all is set when no namespace list or selector narrows the scope.
	all bool
}

// dashboardCoverage is the dashboard's /api/coverage source. It must be
// called after newRunners, which builds the sources.
func (p *pipelineBuild) dashboardCoverage() func() kube.Coverage {
	scoped := p.deps.runtime.Scope()
	c := coverageSources{
		typed: p.source, model: p.model, scope: p.scope,
		all: len(scoped.AllowedNamespaces()) == 0 &&
			scoped.NamespaceSelector() == "",
	}
	if p.dynamic != nil {
		c.dynamic = p.dynamic
	}
	return c.build
}

// build summarises the coverage now.
func (c coverageSources) build() kube.Coverage {
	ids := c.model.Entities(kube.KindNamespace)
	namespaces := make([]string, 0, len(ids))
	for _, id := range ids {
		namespaces = append(namespaces, id.Name)
	}
	in := kube.CoverageInput{
		Typed:       c.typed.WatchedKinds(),
		Unavailable: c.typed.Unavailable(),
		Disabled:    c.typed.Disabled(),
		SyncedAt:    c.typed.SyncedAt(),
		Namespaces:  namespaces,
		InScope: func(namespace string) bool {
			return c.scope.NamespaceAllowed(c.model, namespace)
		},
		All: c.all,
	}
	if c.dynamic != nil {
		in.Dynamic = c.dynamic.Status()
	}
	return kube.BuildCoverage(in)
}
