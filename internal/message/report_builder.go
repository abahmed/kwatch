package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// ReportBuilder constructs a Report from an Incident + Insight.
// It applies context-adaptive field selection: only sections relevant
// to the incident's reason are populated.
type ReportBuilder struct {
	cluster string
}

// NewReportBuilder returns a ReportBuilder with the given cluster name.
func NewReportBuilder(cluster string) *ReportBuilder {
	return &ReportBuilder{
		cluster: cluster,
	}
}

// Build produces a Report from the given incident, action, and optional insight.
func (rb *ReportBuilder) Build(inc *model.Incident, action model.IncidentAction, ins *insight.Insight) *Report {
	r := &Report{
		Action:    actionString(action),
		Reason:    inc.Reason,
		Severity:  string(inc.Severity),
		Resource:  inc.Resource,
		Name:      inc.Name,
		Namespace: inc.Namespace,
		Cluster:   rb.cluster,
		Runbook:   inc.Runbook,
	}

	r.Summary = rb.buildSummary(inc, action)
	rb.populateIdentity(r, inc)
	rb.populateState(r, inc)
	rb.populateDiagnosis(r, inc, ins)
	rb.populateEvidence(r, inc)
	rb.populateChanges(r, ins)
	rb.populateSuppressed(r, inc)
	rb.populateTypeSpecific(r, inc)

	return r
}

func (rb *ReportBuilder) buildSummary(inc *model.Incident, action model.IncidentAction) SummarySection {
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
		r.Identity = &IdentitySection{Node: inc.Name}
		return
	}

	// Pod/container issues
	if inc.Resource == "pod" && inc.ContainerName != "" && inc.ContainerName != "." {
		r.Identity = &IdentitySection{
			Container: inc.ContainerName,
			Image:     inc.Image,
			Node:      inc.NodeName,
			OwnerKind: inc.OwnerKind,
		}
		return
	}

	// Workload issues (deployment, statefulset, etc.)
	r.Identity = &IdentitySection{
		Node:      inc.NodeName,
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

	r.State = &StateSection{
		Message:     inc.LastContainerState.Msg,
		ExitCode:    inc.LastContainerState.ExitCode,
		Restarts:    int32(inc.RestartCount),
		Duration:    durationStr(inc.FirstSeen, inc.LastSeen),
		TotalEvents: inc.Count,
	}
}

func (rb *ReportBuilder) populateDiagnosis(r *Report, inc *model.Incident, ins *insight.Insight) {
	d := &DiagnosisSection{
		Hint: inc.Hint,
	}
	if ins != nil {
		d.Cause = ins.Cause
		d.Impact = ins.Impact
		d.Pattern = ins.Pattern
	}
	r.Diagnosis = d
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
			Resource:  c.Resource,
			Reference: ref,
			Type:      fmt.Sprintf("%v", c.Type),
		})
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
		r.SuppressedPodSummaries = append(r.SuppressedPodSummaries, PodSummaryEntry{
			Namespace:    ps.Namespace,
			PodName:      ps.PodName,
			Reason:       ps.Reason,
			RestartCount: ps.RestartCount,
		})
	}
}

func (rb *ReportBuilder) populateTypeSpecific(r *Report, inc *model.Incident) {
	reason := inc.Reason

	switch {
	case reason == constant.ReasonOOMKilled || reason == constant.ReasonOOMRepeating:
		leakCount, windowMin := extractOOMLeakStats(inc.Hint)
		r.OOM = &OOMSection{
			MemoryLimit: extractMemoryLimit(inc),
			Timeline:    extractOOMTimeline(inc),
			IsLeak:      strings.Contains(inc.Hint, "memory leak"),
			LeakCount:   leakCount,
			WindowMin:   windowMin,
		}
		// Hide exit code (always 137) and image (irrelevant for OOM)
		if r.State != nil {
			r.State.ExitCode = 0
		}
		if r.Identity != nil {
			r.Identity.Image = ""
		}

	case reason == constant.ReasonImagePullBackOff || reason == constant.ReasonErrImagePull:
		r.Image = &ImageSection{
			RegistryHint: extractRegistryHint(inc),
			PullSecrets:  extractPullSecrets(inc),
		}
		// Clear logs (container never started), keep events
		if r.Evidence != nil {
			r.Evidence.Logs = ""
		}
		if r.State != nil {
			r.State.ExitCode = 0
		}

	case reason == constant.ReasonLivenessProbeFailed || reason == constant.ReasonReadinessProbeFailed || reason == constant.ReasonStartupProbeFailed:
		r.Probe = &ProbeSection{
			ProbeType: probeTypeFromReason(reason),
			Endpoint:  extractProbeEndpoint(inc),
		}

	case reason == constant.ReasonUnschedulable || reason == "Pending":
		r.Pending = &PendingSection{
			Delay:            extractSchedulingDelay(inc),
			ResourceRequests: extractResourceRequestStrings(inc),
		}
		// No container identity or evidence for pending pods
		r.Identity = nil
		r.Evidence = nil
	}
}

// --- Helpers ---

func durationStr(first, last time.Time) string {
	d := last.Sub(first).Round(time.Minute)
	if d < time.Minute {
		return "< 1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func actionString(a model.IncidentAction) string {
	switch a {
	case model.ActionCreate:
		return "create"
	case model.ActionUpdate:
		return "update"
	case model.ActionResolved:
		return "resolved"
	default:
		return "unknown"
	}
}

func actionEmoji(action model.IncidentAction, severity model.Severity) string {
	switch action {
	case model.ActionResolved:
		return "✅"
	case model.ActionUpdate:
		return "🔄"
	default:
		switch severity {
		case model.SeverityCritical:
			return "🔴"
		case model.SeverityHigh:
			return "🟠"
		case model.SeverityWarning, model.SeverityMedium:
			return "🟡"
		default:
			return "🔴"
		}
	}
}

func probeTypeFromReason(reason string) string {
	switch reason {
	case constant.ReasonLivenessProbeFailed:
		return "liveness"
	case constant.ReasonReadinessProbeFailed:
		return "readiness"
	case constant.ReasonStartupProbeFailed:
		return "startup"
	}
	return "probe"
}

func reasonLabel(reason string) string {
	switch reason {
	case constant.ReasonOOMKilled:
		return "Out of memory"
	case constant.ReasonCrashLoopBackOff:
		return "Container keeps crashing"
	case constant.ReasonImagePullBackOff, constant.ReasonErrImagePull:
		return "Failed to download image"
	case constant.ReasonLivenessProbeFailed:
		return "Health check failed"
	case constant.ReasonReadinessProbeFailed:
		return "Not ready for traffic"
	case constant.ReasonStartupProbeFailed:
		return "Startup check failed"
	case constant.ReasonUnschedulable:
		return "No capacity available"
	case "Pending":
		return "Waiting for resources"
	case constant.ReasonNodeNotReady:
		return "Node is not ready"
	case constant.ReasonBackOff:
		return "Backing off after crash"
	case constant.ReasonError:
		return "Container error"
	case constant.ReasonHighRestartCount:
		return "Frequent restarts"
	case constant.ReasonInitContainerError:
		return "Init container failed"
	case constant.ReasonOOMRepeating:
		return "Repeated out of memory"
	case constant.ReasonEvicted:
		return "Pod was evicted"
	}
	return reason
}
