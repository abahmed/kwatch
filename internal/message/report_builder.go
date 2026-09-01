package message

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// ReportBuilder constructs a Report from an Incident + Insight.
// It applies context-adaptive field selection: only sections relevant
// to the incident's reason are populated.
type ReportBuilder struct {
	cluster string
	// now is the clock change ages are measured against; injectable for
	// deterministic tests.
	now  func() time.Time
	plan feature.Plan
}

var reportPlan struct {
	sync.RWMutex
	plan feature.Plan
}

// SetFeaturePlan installs the process-wide immutable plan used by all
// provider renderers. Rendering is intentionally centralized here because
// providers build reports through different paths.
func SetFeaturePlan(plan feature.Plan) {
	reportPlan.Lock()
	reportPlan.plan = plan
	reportPlan.Unlock()
}

func currentFeaturePlan() feature.Plan {
	reportPlan.RLock()
	defer reportPlan.RUnlock()
	return reportPlan.plan
}

func reportFeatureEnabled(plan feature.Plan, id feature.ID) bool {
	return len(plan.Decisions) == 0 || plan.Enabled(id)
}

// NewReportBuilder returns a ReportBuilder with the given cluster name.
func NewReportBuilder(cluster string) *ReportBuilder {
	return &ReportBuilder{
		cluster: cluster,
		now:     time.Now,
		plan:    currentFeaturePlan(),
	}
}

// Build produces a Report from the given incident, action, and optional
// insight.
func (rb *ReportBuilder) Build(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) *Report {
	r := &Report{
		Action:      actionString(action),
		Reason:      inc.Reason,
		Severity:    string(inc.Severity),
		Resource:    inc.Resource,
		Name:        inc.Name,
		Namespace:   inc.Namespace,
		Cluster:     rb.cluster,
		Runbook:     inc.Runbook,
		Fingerprint: inc.Fingerprint,
	}

	r.Summary = rb.buildSummary(inc, action)
	rb.populateIdentity(r, inc)
	rb.populateState(r, inc)
	rb.populateDiagnosis(r, inc, ins)
	rb.populateEvidence(r, inc)
	if reportFeatureEnabled(rb.plan, feature.ChangeDiff) {
		rb.populateChanges(r, ins)
	}
	if reportFeatureEnabled(rb.plan, feature.IncidentTimeline) {
		r.Timeline = rb.buildTimeline(inc, ins, action)
	}
	rb.populateSuppressed(r, inc)
	rb.populateTypeSpecific(r, inc)

	return r
}

func (rb *ReportBuilder) buildSummary(
	inc *model.Incident,
	action model.IncidentAction,
) SummarySection {
	return SummarySection{
		Emoji:    actionEmoji(action, inc.Severity),
		Duration: durationStr(inc.FirstSeen, inc.LastSeen),
		Count:    inc.Count,
		Peak:     inc.PeakResources,
		Label:    reasonLabel(inc.Reason),
	}
}

func (rb *ReportBuilder) populateIdentity(r *Report, inc *model.Incident) {
	// Pending/Unschedulable: no container identity
	if r.Reason == constant.ReasonUnschedulable || r.Reason == "Pending" {
		return
	}

	// Node issues: only node identity
	if inc.Resource == "node" {
		r.Identity = &IdentitySection{Node: format.ShortNode(inc.Name)}
		return
	}

	// Pod/container issues
	if inc.Resource == "pod" && inc.ContainerName != "" &&
		inc.ContainerName != "." {
		r.Identity = &IdentitySection{
			Container: inc.ContainerName,
			Image:     format.ShortImage(inc.Image),
			Node:      format.ShortNode(inc.NodeName),
			OwnerKind: inc.OwnerKind,
		}
		return
	}

	// Workload issues (deployment, statefulset, etc.)
	r.Identity = &IdentitySection{
		Node:      format.ShortNode(inc.NodeName),
		OwnerKind: inc.OwnerKind,
	}
}

func (rb *ReportBuilder) populateState(r *Report, inc *model.Incident) {
	// Pending: no container state
	if r.Reason == constant.ReasonUnschedulable || r.Reason == "Pending" {
		return
	}

	if inc.LastContainerState == nil {
		return
	}

	restarts := inc.RestartCount
	if restarts > int(^uint32(0)>>1) {
		restarts = int(^uint32(0) >> 1)
	}
	r.State = &StateSection{
		Message:     inc.LastContainerState.Msg,
		ExitCode:    inc.LastContainerState.ExitCode,
		Restarts:    int32(restarts),
		Duration:    durationStr(inc.FirstSeen, inc.LastSeen),
		TotalEvents: inc.Count,
	}
}

func (rb *ReportBuilder) populateDiagnosis(
	r *Report,
	inc *model.Incident,
	ins *insight.Insight,
) {
	d := &DiagnosisSection{
		Hint: dedupeHint(inc.Hint, r.Name, r.Summary.Label, stateMessage(inc)),
	}
	if ins != nil {
		if reportFeatureEnabled(rb.plan, feature.DirectDiagnosis) {
			d.Cause = ins.Cause
			d.Pattern = ins.Pattern
			d.NextSteps = append([]string(nil), ins.NextSteps...)
		}
		if reportFeatureEnabled(rb.plan, feature.ImpactAnalysis) {
			d.Impact = ins.Impact
		}
		if reportFeatureEnabled(rb.plan, feature.RCAConfidence) {
			d.Confidence = ins.Confidence
			d.Evidence = append([]string(nil), ins.Evidence...)
		}
	}
	// Topology the correlation engine resolved from live Service selectors.
	// It is impact, and belongs with the rest of the impact.
	if reportFeatureEnabled(rb.plan, feature.ImpactAnalysis) && d.Impact == "" && len(inc.AffectedServices) > 0 {
		label := "service"
		if len(inc.AffectedServices) > 1 {
			label = "services"
		}
		d.Impact = "affects " + label + " " + format.JoinNames(
			inc.AffectedServices,
			4,
		)
	}
	if reportFeatureEnabled(rb.plan, feature.DirectDiagnosis) && inc.OwnerUnhealthy && inc.OwnerKind != "" && d.Cause == "" {
		d.Cause = fmt.Sprintf(
			"owning %s is unhealthy — this looks like a rollout, not an "+
				"isolated crash",
			inc.OwnerKind,
		)
		d.Pattern = "rollout_failure"
	}
	r.Diagnosis = d
}

func stateMessage(inc *model.Incident) string {
	if inc.LastContainerState == nil {
		return ""
	}
	return inc.LastContainerState.Msg
}

// dedupeHint removes hint fragments that merely repeat text shown elsewhere
// in the message: the incident's own name, its human label, or the state
// message. The hint is for what is *not* already on screen.
func dedupeHint(hint string, shown ...string) string {
	if strings.TrimSpace(hint) == "" {
		return ""
	}
	var keep []string
	seen := make(map[string]bool)
	for _, frag := range strings.Split(hint, "; ") {
		frag = strings.TrimSpace(
			strings.TrimRight(strings.TrimSpace(frag), "—-;: "),
		)
		if frag == "" || seen[frag] {
			continue
		}
		redundant := false
		for _, sh := range shown {
			sh = strings.TrimSpace(sh)
			if sh == "" {
				continue
			}
			if frag == sh || strings.Contains(sh, frag) ||
				strings.HasPrefix(frag, sh) {
				redundant = true
				break
			}
		}
		if redundant {
			continue
		}
		seen[frag] = true
		keep = append(keep, frag)
	}
	return strings.Join(keep, "; ")
}

func (rb *ReportBuilder) populateEvidence(r *Report, inc *model.Incident) {
	if inc.Logs == "" && inc.Events == "" {
		return
	}
	if !inc.IncludeLogs && !inc.IncludeEvents {
		return
	}

	r.Evidence = &EvidenceSection{}
	if inc.IncludeLogs {
		r.Evidence.Logs = inc.Logs
	}
	if inc.IncludeEvents {
		r.Evidence.Events = inc.Events
	}
}

func (rb *ReportBuilder) populateChanges(r *Report, ins *insight.Insight) {
	if ins == nil || len(ins.RecentChanges) == 0 {
		return
	}

	items := make([]ChangeItem, 0, len(ins.RecentChanges))
	seen := make(map[string]bool)
	for _, c := range ins.RecentChanges {
		key := c.Resource + "/" + c.Namespace + "/" + c.Name + c.Type.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		ref := c.Name
		if c.Namespace != "" {
			ref = c.Namespace + "/" + c.Name
		}
		items = append(items, ChangeItem{
			Resource:   c.Resource,
			Reference:  ref,
			Type:       fmt.Sprintf("%v", c.Type),
			Age:        ageOf(c.Timestamp, rb.now()),
			Additional: c.Additional,
		})
		for _, field := range c.Fields {
			items[len(items)-1].Fields = append(items[len(items)-1].Fields, FieldChange{Path: field.Path, Before: field.Before, After: field.After, Action: field.Action})
		}
	}

	if len(items) > 0 {
		r.Changes = &ChangesSection{
			Items:         items,
			AffectedCount: ins.AffectedCount,
		}
	}
}

func (rb *ReportBuilder) populateSuppressed(r *Report, inc *model.Incident) {
	if inc.SuppressedPods == 0 {
		return
	}
	r.SuppressedPods = inc.SuppressedPods
	for _, ps := range inc.SuppressedPodSummaries {
		r.SuppressedPodSummaries = append(
			r.SuppressedPodSummaries,
			PodSummaryEntry{
				Namespace:    ps.Namespace,
				PodName:      ps.PodName,
				Reason:       ps.Reason,
				RestartCount: ps.RestartCount,
			},
		)
	}
}

func (rb *ReportBuilder) populateTypeSpecific(r *Report, inc *model.Incident) {
	reason := inc.Reason

	facts := inc.Facts
	switch {
	case reason == constant.ReasonOOMKilled ||
		reason == constant.ReasonOOMRepeating:
		r.OOM = &OOMSection{
			MemoryLimit: facts.MemoryLimit,
			Timeline:    facts.OOMTimeline,
			IsLeak:      facts.MemoryLeak,
			LeakCount:   facts.OOMCount,
			WindowMin:   facts.OOMWindowMin,
		}
		// Hide exit code (always 137) and image (irrelevant for OOM)
		if r.State != nil {
			r.State.ExitCode = 0
		}
		if r.Identity != nil {
			r.Identity.Image = ""
		}

	case reason == constant.ReasonImagePullBackOff ||
		reason == constant.ReasonErrImagePull:
		r.Image = &ImageSection{
			RegistryHint: registryHint(inc),
			PullSecrets:  facts.PullSecretsSet,
		}
		// Clear logs (container never started), keep events
		if r.Evidence != nil {
			r.Evidence.Logs = ""
		}
		if r.State != nil {
			r.State.ExitCode = 0
		}

	case reason == constant.ReasonLivenessProbeFailed ||
		reason == constant.ReasonReadinessProbeFailed ||
		reason == constant.ReasonStartupProbeFailed:
		r.Probe = &ProbeSection{
			ProbeType: probeTypeFromReason(reason),
			Endpoint:  facts.ProbeEndpoint,
		}

	case reason == constant.ReasonUnschedulable || reason == "Pending":
		r.Pending = &PendingSection{
			ResourceRequests: facts.ResourceRequests,
		}
		if facts.SchedulingDelay > 0 {
			r.Pending.Delay = format.Duration(facts.SchedulingDelay)
		}
		// No container identity or evidence for pending pods
		r.Identity = nil
		r.Evidence = nil
	}
}
