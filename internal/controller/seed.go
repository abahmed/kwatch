package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// baselineRecorder accumulates the startup Seen set. add converts from the
// typed incident key to the raw ConfigMap wire format; seed handles owner-level
// signals that carry a PodName.
type baselineRecorder struct {
	now        time.Time
	baseline   map[string]map[string]int64
	suppressed map[string]int
	total      int
	max        int
}

func newBaselineRecorder(now time.Time, max int) *baselineRecorder {
	return &baselineRecorder{
		now:        now,
		baseline:   make(map[string]map[string]int64),
		suppressed: map[string]int{},
		max:        max,
	}
}

func (r *baselineRecorder) add(key model.IncidentKey, pod string) {
	ks := string(key)
	if r.total >= r.max {
		return
	}
	if r.baseline[ks] == nil {
		r.baseline[ks] = map[string]int64{}
	}
	if _, exists := r.baseline[ks][pod]; !exists {
		r.total++
	}
	r.baseline[ks][pod] = r.now.Unix()

	if pk := correlation.ParseKey(key); pk.Owner != "" {
		r.suppressed[pk.Owner+"/"+pk.Reason]++
	}
}

// seed records a detector's signal in the baseline under the key the live
// path will produce for it.
//
// Both of these used to hand-copy a subset of the signal into an event, and
// what they left out changed the key: no Message (which decides whether an
// image-pull failure is keyed globally) and no container state (which decides
// whether a repeatedly-restarting container folds to one crash-loop key). A
// key that differs from the live one is a baseline entry that suppresses
// nothing. The signal's own conversion is used instead.
func (r *baselineRecorder) seed(obs *model.Observation) {
	// Only a pod subject gets a pod-level entry. A Deployment, an Ingress or
	// a Node is seeded at owner level -- the empty pod key -- which covers
	// every pod of that owner and expires on the short owner TTL instead of
	// the full baseline TTL. Detectors used to disagree about this purely by
	// whether they happened to fill in a name.
	pod := ""
	if obs.Subject.Kind == "pod" {
		pod = obs.Subject.Name
	}
	r.add(correlation.ObservationKey(obs), pod)
}

// seedControlPlane records CP signals under the actual pod name.
func (r *baselineRecorder) seedControlPlane(
	pod *corev1.Pod, obs *model.Observation,
) {
	r.add(correlation.ObservationKey(obs), pod.Name)
}

// buildSeenSet records what was already broken when kwatch started, so those
// problems are reported once as a summary instead of as a burst of new
// incidents.
//
// A seed means "this was failing at startup", which is not the same as "this
// would alert": the detectors run here without the sustain gating the live
// path applies, so something failing for one minute of a five-minute sustain
// window is seeded even though no incident would have opened. Owner-level
// entries therefore expire on the engine's short OwnerBaselineTTL rather than
// the full baseline TTL, and any observed recovery retires them.
func (c *Controller) buildSeenSet() {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for Seen set")
		return
	}

	rec := newBaselineRecorder(c.nowTime(), c.maxBaseline)

	podListers := handler.Listers{
		Secret:         c.secretLister,
		ConfigMap:      c.configMapLister,
		ServiceAccount: c.serviceAccountLister,
	}
	for _, pod := range pods {
		c.emitBaseline(rec, pod)
		if sig := handler.DetectPodDeletionIssue(pod, c.nowTime()); sig != nil {
			rec.seed(sig)
		}
		for _, sig := range handler.DetectPodReferenceIssues(pod, podListers) {
			rec.seed(sig)
		}
	}

	c.seedNodeBaseline()
	c.seedControllers(rec)
	c.seedServices(rec)
	c.seedControllersWithSvc(rec)
	c.seedControlPlaneBaseline(rec)

	if len(rec.baseline) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(rec.baseline))
		c.handler.SetBaseline(rec.baseline)
	}
	c.handler.ReportStartupSummary(rec.suppressed)
}

// emitBaseline records container, scheduling and node issues for one pod.
func (c *Controller) emitBaseline(rec *baselineRecorder, pod *corev1.Pod) {
	if pod.Status.Phase == corev1.PodSucceeded {
		return
	}
	owner := observe.PodOwners{
		RS: c.rsLister, DS: c.dsLister, SS: c.ssLister,
	}.OwnerOf(pod)
	if owner.Name == "" {
		return
	}

	statuses := make([]corev1.ContainerStatus, 0,
		len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	statuses = append(statuses, pod.Status.InitContainerStatuses...)

	hadContainerIssue := false
	for _, cs := range statuses {
		reason := containerIssueReason(&cs)
		if reason == "" {
			continue
		}
		obs := observe.PodOwnedBy(pod, cs.Name, reason, owner)
		obs.Message = filter.ContainerIssueMessage(&cs)
		obs.RestartCount = cs.RestartCount
		rec.add(correlation.ObservationKey(obs), pod.Name)
		hadContainerIssue = true
	}

	if hadContainerIssue {
		return
	}
	if reason := c.podLevelSeedReason(pod); reason != "" {
		obs := observe.PodOwnedBy(pod, ".", reason, owner)
		rec.add(correlation.ObservationKey(obs), pod.Name)
	}
}

// podLevelSeedReason is the pod-level reason the live pipeline would report
// for this pod right now, or "" when it would report none.
//
// Only the scheduling condition used to be seeded. A pod that had been
// unready for three hours, or Pending with no condition explaining why, was
// not in the baseline at all: the live detectors suppress themselves for one
// threshold after startup, and then announced a problem that predated the
// restart as brand new, with a duration measured from the restart.
func (c *Controller) podLevelSeedReason(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled &&
			cond.Status == corev1.ConditionFalse {
			return cond.Reason
		}
	}
	now := c.nowTime()
	th := c.seedThresholds
	if pod.Status.Phase == corev1.PodPending && th.pendingPodEnabled {
		ref := pod.CreationTimestamp.Time
		if !ref.IsZero() && now.Sub(ref) >= th.pendingPod {
			return constant.ReasonPodPending
		}
		return ""
	}
	if pod.Status.Phase != corev1.PodRunning || !th.notReadyEnabled {
		return ""
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type != corev1.PodReady {
			continue
		}
		if cond.Status == corev1.ConditionTrue ||
			cond.LastTransitionTime.IsZero() {
			return ""
		}
		if now.Sub(cond.LastTransitionTime.Time) >= th.notReady {
			return constant.ReasonContainersNotReady
		}
		return ""
	}
	return ""
}

// containerIssueReason is the shared rule the live container detector uses,
// so a seeded baseline key matches the key the live signal will produce.
func containerIssueReason(cs *corev1.ContainerStatus) string {
	return filter.ContainerIssueReason(cs)
}

// seedNodeBaseline pre-populates the active node incidents so pod suppression
// is in force before any worker starts.
//
// Nothing is written to the baseline here, and that is deliberate. The engine
// exempts node events from the baseline check outright -- a node that was
// already down at startup must still be announced, because every pod on it
// depends on that alert existing -- so a node entry suppressed nothing. What
// it did do was consume one of the bounded baseline slots that a real
// suppression needed, and count itself into the startup summary, which then
// told the operator kwatch had quietly absorbed problems it went on to alert
// about anyway.
func (c *Controller) seedNodeBaseline() {
	if len(c.node.synced) == 0 || c.nodeLister == nil {
		return
	}
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list nodes for baseline seeding")
		return
	}
	var activeNodeIncidents []string
	for _, n := range nodes {
		for _, cond := range n.Status.Conditions {
			if handler.NodeConditionReason(cond) == "" {
				continue
			}
			activeNodeIncidents = append(activeNodeIncidents, n.Name)
			break
		}
	}
	if len(activeNodeIncidents) > 0 {
		c.handler.SetActiveNodeIncidents(activeNodeIncidents)
	}
}
