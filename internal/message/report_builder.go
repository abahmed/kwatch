package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
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
	now func() time.Time
}

// NewReportBuilder returns a ReportBuilder with the given cluster name.
func NewReportBuilder(cluster string) *ReportBuilder {
	return &ReportBuilder{
		cluster: cluster,
		now:     time.Now,
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

	r.State = &StateSection{
		Message:     inc.LastContainerState.Msg,
		ExitCode:    inc.LastContainerState.ExitCode,
		Restarts:    int32(inc.RestartCount),
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
		d.Cause = ins.Cause
		d.Impact = ins.Impact
		d.Pattern = ins.Pattern
	}
	// Topology the correlation engine resolved from live Service selectors.
	// It is impact, and belongs with the rest of the impact.
	if d.Impact == "" && len(inc.AffectedServices) > 0 {
		label := "service"
		if len(inc.AffectedServices) > 1 {
			label = "services"
		}
		d.Impact = "affects " + label + " " + format.JoinNames(
			inc.AffectedServices,
			4,
		)
	}
	if inc.OwnerUnhealthy && inc.OwnerKind != "" && d.Cause == "" {
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
			Resource:  c.Resource,
			Reference: ref,
			Type:      fmt.Sprintf("%v", c.Type),
			Age:       ageOf(c.Timestamp, rb.now()),
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

// --- Helpers ---

// registryHint is the kubelet's own message for an image-pull failure, e.g.
// `Back-off pulling image "ghcr.io/x/y:v2"` — the most specific thing we know.
func registryHint(inc *model.Incident) string {
	if inc.LastContainerState == nil {
		return ""
	}
	return inc.LastContainerState.Msg
}

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
		// Red is reserved for critical. A normal alert that shows the same
		// colour as a critical one teaches readers to ignore the colour.
		switch severity {
		case model.SeverityCritical:
			return "🔴"
		case model.SeverityHigh:
			return "🟠"
		case model.SeverityWarning, model.SeverityMedium:
			return "🟡"
		default:
			return "🔵"
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

// reasonLabels maps a Kubernetes reason code to the phrase a person would use
// for it. The code stays visible on the headline for searching; this is what
// the reader parses first.
var reasonLabels = map[string]string{
	constant.ReasonOOMKilled:            "Out of memory",
	constant.ReasonCrashLoopBackOff:     "Container keeps crashing",
	constant.ReasonImagePullBackOff:     "Failed to download image",
	constant.ReasonErrImagePull:         "Failed to download image",
	constant.ReasonLivenessProbeFailed:  "Health check failed",
	constant.ReasonReadinessProbeFailed: "Not ready for traffic",
	constant.ReasonStartupProbeFailed:   "Startup check failed",
	constant.ReasonUnschedulable:        "No capacity available",
	"Pending":                           "Waiting for resources",
	constant.ReasonNodeNotReady:         "Node is not ready",
	constant.ReasonBackOff:              "Backing off after crash",
	constant.ReasonError:                "Container error",
	constant.ReasonHighRestartCount:     "Frequent restarts",
	constant.ReasonInitContainerError:   "Init container failed",
	constant.ReasonOOMRepeating:         "Repeated out of memory",
	constant.ReasonEvicted:              "Pod was evicted",
	constant.ReasonContainersNotReady:   "Pod not ready",

	// workloads
	constant.ReasonDeploymentUnavailable: "Deployment below desired replicas",

	constant.ReasonDaemonSetUnavailable: "DaemonSet below desired replicas",

	constant.ReasonStsUnavailable: "StatefulSet below desired replicas",

	constant.ReasonProgressDeadlineExceeded: "Rollout stuck",

	constant.ReasonHPAMaxedOut: "Autoscaler pinned at max replicas",

	constant.ReasonHPAScalingError:     "Autoscaler cannot scale",
	constant.ReasonJobFailed:           "Job failed",
	constant.ReasonJobSuspended:        "Job suspended",
	constant.ReasonCronJobSuspended:    "CronJob suspended",
	constant.ReasonCronJobNotScheduled: "CronJob missed its schedule",
	constant.ReasonPdbViolation:        "Disruption budget violated",

	// networking
	constant.ReasonServiceNoEndpoints:     "Service has no healthy backends",
	constant.ReasonIngressBackendNotFound: "Ingress points at missing service",
	constant.ReasonWebhookBackendNotFound: "Admission webhook has no backend",
	constant.ReasonTLSCertExpired:         "TLS certificate expired",
	constant.ReasonTLSCertExpiringSoon:    "TLS certificate expiring soon",

	// cluster
	constant.ReasonControlPlaneComponentFailure: "Control-plane component failing",

	constant.ReasonVolumeUsageHigh:      "Volume running out of space",
	constant.ReasonNodeResourceHigh:     "Node overcommitted",
	constant.ReasonNodeResourceCritical: "Node overcommitted",
	constant.ReasonMemoryPressure:       "Node under memory pressure",
	constant.ReasonNodeMemoryPressure:   "Node under memory pressure",
	constant.ReasonDiskPressure:         "Node under disk pressure",
	constant.ReasonPIDPressure:          "Node out of process IDs",
	constant.ReasonNetworkUnavailable:   "Node network unavailable",
	constant.ReasonContainerCannotRun:   "Container cannot start",
	constant.ReasonCreateContainerError: "Container cannot start",
	constant.ReasonCreateConfigError:    "Missing ConfigMap or Secret",
	constant.ReasonDeadlineExceeded:     "Ran past its deadline",
	constant.ReasonImageInspectError:    "Invalid image reference",
	constant.ReasonInvalidImageName:     "Invalid image reference",
	constant.ReasonSandboxError:         "Pod sandbox failed",
	constant.ReasonPreempting:           "Pod preempted",

	constant.ReasonPreExistingAtStartup: "Already broken when kwatch started",
}

// reasonLabel returns the human phrase for a reason, or the reason itself
// when there is none.
func reasonLabel(reason string) string {
	if label, ok := reasonLabels[reason]; ok {
		return label
	}
	return reason
}

// ageOf renders how long ago t was, coarsely — "40s", "3m", "2h". Empty for
// the zero time.
func ageOf(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
