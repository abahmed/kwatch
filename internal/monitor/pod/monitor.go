package pod

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

// Monitor owns the ordered pod and container detection policy. It does not
// perform Kubernetes I/O or decide whether an incident is delivered.
type Monitor struct {
	podDetectors                  []policy.Detector
	podEnrichers                  []enrichment.Enricher
	containerDetectors            []policy.Detector
	containerSuppressionEnrichers []enrichment.Enricher
	containerDataEnrichers        []enrichment.Enricher
}

// NewWithRuntimeConfig constructs pod policy from the immutable runtime
// snapshot.
func NewWithRuntimeConfig(runtime config.RuntimeConfig) *Monitor {
	containerDetectors := BuildContainerDetectorsWithRuntimeConfig(runtime)
	containerSuppressionEnrichers := BuildContainerSuppressionEnrichers()
	return &Monitor{
		podDetectors:                  BuildPodDetectorsWithRuntimeConfig(runtime),
		podEnrichers:                  BuildPodEnrichers(),
		containerDetectors:            containerDetectors,
		containerSuppressionEnrichers: containerSuppressionEnrichers,
		containerDataEnrichers:        BuildContainerDataEnrichers(),
	}
}

// DetectPod runs the pure pod detector stage. It returns false when the pod
// should not continue through pod-level enrichment.
func (m *Monitor) DetectPod(ctx *enrichment.Context) bool {
	policyCtx := toPolicyContext(ctx)
	for _, detector := range m.podDetectors {
		if detector.Detect(policyCtx) == policy.DecisionSuppress {
			copyPolicyFindings(ctx, policyCtx)
			return false
		}
	}
	copyPolicyFindings(ctx, policyCtx)
	return ctx.PodHasIssues && !ctx.ContainersHasIssues
}

// EnrichPod runs the pod enrichment stage and reports whether processing must
// stop because an enricher suppressed the finding.
func (m *Monitor) EnrichPod(ctx *enrichment.Context) bool {
	for _, enricher := range m.podEnrichers {
		if enricher.Enrich(ctx) {
			return true
		}
	}
	return false
}

// DetectContainer runs the pure container detector stage.
func (m *Monitor) DetectContainer(ctx *enrichment.Context) bool {
	policyCtx := toPolicyContext(ctx)
	for _, detector := range m.containerDetectors {
		if detector.Detect(policyCtx) == policy.DecisionSuppress {
			copyPolicyFindings(ctx, policyCtx)
			return false
		}
	}
	copyPolicyFindings(ctx, policyCtx)
	return len(m.containerDetectors) > 0
}

// SuppressContainer runs enrichers that may suppress a container finding.
func (m *Monitor) SuppressContainer(ctx *enrichment.Context) bool {
	for _, enricher := range m.containerSuppressionEnrichers {
		if enricher.Enrich(ctx) {
			return true
		}
	}
	return false
}

// EnrichContainerData runs enrichers whose data is useful even when the
// current finding is suppressed.
func (m *Monitor) EnrichContainerData(ctx *enrichment.Context) {
	for _, enricher := range m.containerDataEnrichers {
		enricher.Enrich(ctx)
	}
}

func toPolicyContext(ctx *enrichment.Context) *policy.Context {
	return &policy.Context{
		Pod:       ctx.Pod,
		EvType:    ctx.EvType,
		Runtime:   ctx.Runtime,
		Now:       ctx.Now,
		Findings:  ctx.Findings,
		Container: ctx.Container,
	}
}

func copyPolicyFindings(
	dst *enrichment.Context,
	src *policy.Context,
) {
	dst.Findings = src.Findings
	if dst.Container == nil {
		dst.Container = src.Container
	}
}
