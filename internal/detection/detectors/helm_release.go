package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultHelmPendingHold is how long the latest revision of a Helm
// release may stay pending or uninstalling before it counts as stuck.
// An install or upgrade that waits for its pods (--wait) is pending for
// minutes as a matter of course.
const DefaultHelmPendingHold = 15 * time.Minute

// DefaultHelmFailureOld is the age past which a failed revision whose
// workloads are healthy is old news: the default old-problem horizon
// (oldProblemDays, 30 days). Such a release is Low, so the incident
// layer lists it with the old problems instead of raising a warning on
// the day kwatch is installed.
const DefaultHelmFailureOld = 30 * 24 * time.Hour

// helmWorkloadsShown bounds the workloads of a release named as context.
const helmWorkloadsShown = 3

// HelmRelease detects a Helm release whose latest revision failed, or
// has stayed pending-install, pending-upgrade, pending-rollback or
// uninstalling for longer than the hold. It reads only the labels Helm
// writes on its release Secrets. A deployed, superseded or uninstalled
// release raises nothing.
type HelmRelease struct {
	hold time.Duration
	old  time.Duration
}

// NewHelmRelease returns the detector; a hold of zero or less uses
// DefaultHelmPendingHold.
func NewHelmRelease(hold time.Duration) HelmRelease {
	if hold <= 0 {
		hold = DefaultHelmPendingHold
	}
	return HelmRelease{hold: hold, old: DefaultHelmFailureOld}
}

// Name implements detection.Detector.
func (HelmRelease) Name() string { return "helm-release" }

// Kinds implements detection.Detector.
func (HelmRelease) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindHelmRelease}
}

// Detect implements detection.Detector.
func (h HelmRelease) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	// A release is known only from the release Secret watch, which
	// reads Secrets (no access of its own): until it has synced, or
	// with Secrets unwatched, nothing is said.
	if !ctx.Readable(kube.KindHelmRelease) {
		return nil
	}
	status := text(e, kube.AttrHelmStatus)
	revision, ok := number(e, kube.AttrHelmRevision)
	since := timestamp(e, kube.AttrHelmStatusSince)
	if !ok || since.IsZero() {
		return nil
	}
	at := " at revision " + strconv.Itoa(int(revision))
	var f detection.Finding
	switch {
	case status == "failed":
		f = detection.Finding{Reason: reasons.HelmReleaseFailed,
			Severity: h.failedSeverity(ctx, e.ID, since),
			Summary: "Helm release has failed to " +
				helmAction(revision) + at}
	case helmPending(status):
		waited := ctx.Now.Sub(since)
		if waited < h.hold {
			ctx.RecheckAfter(h.hold - waited)
			return nil
		}
		f = detection.Finding{Reason: reasons.HelmReleaseStuck,
			Severity: uninstallSeverity(ctx, e.ID, status, since),
			Summary: "Helm release has been stuck in " + status + at +
				" since " + since.UTC().Format("15:04 MST") +
				"; Helm refuses another upgrade of it while it is " +
				status}
	default:
		return nil
	}
	f.Since = since
	f.Evidence = helmEvidence(ctx, e, status, revision)
	return []detection.Finding{f}
}

// failedSeverity is Warning, or Low (Info) for a failure older than the
// old-problem horizon while every workload the release deploys is
// healthy or it deploys none kwatch sees. A younger failure is judged
// again when it comes of age.
func (h HelmRelease) failedSeverity(
	ctx detection.Context, release inventory.EntityID, since time.Time,
) detection.Severity {
	if age := ctx.Now.Sub(since); age <= h.old {
		ctx.RecheckAfter(h.old - age + time.Second)
		return detection.Warning
	}
	if deployedFailing(ctx, release) {
		return detection.Warning
	}
	return detection.Info
}

// uninstallSeverity is Warning, or Low (Info) for an uninstall stuck for
// DefaultLeftoverAge that left no workload of the release behind: what
// remains is the record of a removed installation, left over like the
// rest of what it left (incident/leftover_link.go).
func uninstallSeverity(
	ctx detection.Context, release inventory.EntityID, status string,
	since time.Time,
) detection.Severity {
	if status != "uninstalling" {
		return detection.Warning
	}
	if age := ctx.Now.Sub(since); age < DefaultLeftoverAge {
		ctx.RecheckAfter(DefaultLeftoverAge - age + time.Second)
		return detection.Warning
	}
	if len(helmWorkloads(ctx.Model, release)) > 0 {
		return detection.Warning
	}
	return detection.Info
}

// deployedFailing reports a serving workload the release deploys
// (inventory.DeployedBy) with fewer ready replicas than it wants, or
// with a pod unready for DefaultUnavailable (a rollout whose new pod
// crashes while the old one serves). A CronJob is not judged: its
// failed runs are findings of their own.
func deployedFailing(
	ctx detection.Context, release inventory.EntityID,
) bool {
	model := ctx.Model
	for _, id := range model.Related(release, inventory.DeployedBy,
		inventory.Incoming) {
		want := kube.AttrReplicas
		switch id.Kind {
		case kube.KindDaemonSet:
			want = kube.AttrDesiredReplicas
		case kube.KindDeployment, kube.KindStatefulSet:
		default:
			continue
		}
		desired, ok := model.Attribute(id, want)
		ready, _ := model.Attribute(id, kube.AttrReadyReplicas)
		d, _ := desired.Value.AsNumber()
		r, _ := ready.Value.AsNumber()
		if ok && r < d || podUnready(ctx, id) {
			return true
		}
	}
	return false
}

// podUnready reports a running pod of workload that has not been ready
// for DefaultUnavailable; a shorter wait asks for a recheck.
func podUnready(ctx detection.Context, workload inventory.EntityID) bool {
	for _, pod := range ctx.Model.EntitiesIn(kube.KindPod,
		workload.Namespace) {
		ready, ok := flagOf(ctx.Model, pod, kube.AttrReady)
		if !ok || ready || podFinishedID(ctx.Model, pod) ||
			inventory.TopOwner(ctx.Model, pod) != workload {
			continue
		}
		since, _ := ctx.Model.Attribute(pod, kube.AttrReadySince)
		if sustained(ctx, "helm-unready/"+pod.String(),
			since.Value.AsTime(), DefaultUnavailable) {
			return true
		}
	}
	return false
}

// helmPending reports a status Helm sets while an operation runs.
func helmPending(status string) bool {
	return strings.HasPrefix(status, "pending-") || status == "uninstalling"
}

// helmAction is what a failed revision tried: the first revision is an
// install, a later one an upgrade or a rollback.
func helmAction(revision float64) string {
	if revision <= 1 {
		return "install"
	}
	return "upgrade or roll back"
}

// helmEvidence states the release, its revisions and the workloads it
// deploys, as recorded by their release marks.
func helmEvidence(
	ctx detection.Context, e inventory.Entity, status string,
	revision float64,
) []detection.Evidence {
	out := []detection.Evidence{
		{Label: detection.EvidenceHelmRelease,
			Value: e.ID.Namespace + "/" + e.ID.Name},
		{Label: detection.EvidenceHelmRevision,
			Value: strconv.Itoa(int(revision))},
		{Label: detection.EvidenceHelmStatus, Value: status},
	}
	if good, ok := number(e, kube.AttrHelmGoodRevision); ok && good > 0 {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceHelmGoodRevision,
			Value: strconv.Itoa(int(good))})
	}
	for _, w := range helmWorkloads(ctx.Model, e.ID) {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceHelmWorkload, Value: w})
	}
	return out
}

// helmWorkloads are the workloads of the release's namespace whose
// release marks name it, as "deployment/payments".
func helmWorkloads(
	model inventory.Reader, release inventory.EntityID,
) []string {
	var out []string
	kinds := append([]inventory.Kind{kube.KindCronJob},
		kube.WorkloadKinds...)
	for _, kind := range kinds {
		for _, id := range model.Entities(kind) {
			if id.Namespace != release.Namespace {
				continue
			}
			w, ok := model.Entity(id)
			if ok && releaseNamed(w, release.Name) {
				out = append(out, string(kind)+"/"+id.Name)
			}
		}
	}
	sort.Strings(out)
	return out[:min(len(out), helmWorkloadsShown)]
}

// releaseNamed reports an entity whose release marks include name.
func releaseNamed(e inventory.Entity, name string) bool {
	for _, mark := range strings.Split(text(e, kube.AttrRelease), ",") {
		if mark == name {
			return true
		}
	}
	return false
}
