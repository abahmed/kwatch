package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// readinessWorld is pod shop/api-1 with one container app in the given
// state, and the Community container and pod detectors that withhold
// its readiness findings.
type readinessWorld struct {
	model     *inventory.Model
	pod, app  inventory.EntityID
	withheld  *feature.Withheld
	container restricted
	podDet    restricted
}

func newReadinessWorld(
	t *testing.T, state map[string]inventory.Value,
) readinessWorld {
	t.Helper()
	w := readinessWorld{model: newTestModel(),
		pod:      newID(kube.KindPod, "shop", "api-1"),
		app:      newID(kube.KindContainer, "shop", "api-1/app"),
		withheld: feature.NewWithheld()}
	observeEntity(w.model, w.pod, movedAt, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running")})
	observeEntity(w.model, w.app, movedAt, state)
	relateEntity(w.model, w.app, inventory.PartOf, w.pod)
	w.container, _ = editionPair(fixed{Detector: Container{},
		found: findingsOf(reasons.ReadinessProbeFailed)},
		containerGoverns, w.withheld)
	w.podDet, _ = editionPair(fixed{Detector: NewPod(PodThresholds{}),
		found: findingsOf(reasons.ContainersNotReady)},
		podGoverns, w.withheld)
	return w
}

// detectBoth runs the container and the pod detector once each.
func (w readinessWorld) detectBoth() int {
	ctx := testDetectorContext(w.model, movedAt)
	app, _ := w.model.Entity(w.app)
	pod, _ := w.model.Entity(w.pod)
	w.container.Detect(ctx, app)
	w.podDet.Detect(ctx, pod)
	return w.withheld.Counts(movedAt)[feature.WithheldReadiness]
}

func running() map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(false)}
}

func TestWithheldReadinessCountsOncePerPod(t *testing.T) {
	w := newReadinessWorld(t, running())
	assert.Equal(t, 1, w.detectBoth(),
		"the container's probe and the pod's readiness are one pod")
}

func TestWithheldReadinessSkipsAPodThatFailsOnItsOwn(t *testing.T) {
	restarted := running()
	restarted[kube.AttrRestarts] = inventory.Number(4)
	restarted[kube.AttrLastFinished] = inventory.Time(
		movedAt.Add(-2 * time.Minute))
	cases := map[string]map[string]inventory.Value{
		"crash loop": {
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)},
		"never starts": {
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.CreateConfigError)},
		"exited": {
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrStateReason: inventory.Text("Error"),
			kube.AttrExitCode:    inventory.Number(1)},
		"running between restarts": restarted,
	}
	for name, state := range cases {
		t.Run(name, func(t *testing.T) {
			w := newReadinessWorld(t, state)
			assert.Zero(t, w.detectBoth())
		})
	}
}

func TestWithheldReadinessCountsAfterAnOldRestart(t *testing.T) {
	old := running()
	old[kube.AttrRestarts] = inventory.Number(1)
	old[kube.AttrLastFinished] = inventory.Time(movedAt.Add(-time.Hour))
	w := newReadinessWorld(t, old)
	assert.Equal(t, 1, w.detectBoth())
}

func TestWithheldReadinessSkipsAnEntityWithACommunityFinding(t *testing.T) {
	withheld := feature.NewWithheld()
	pod, _ := editionPair(fixed{Detector: NewPod(PodThresholds{}),
		found: findingsOf(reasons.PodFailed, reasons.ContainersNotReady)},
		podGoverns, withheld)
	e := inventory.Entity{ID: newID(kube.KindPod, "shop", "api-1")}
	pod.Detect(detection.Context{Now: movedAt}, e)
	assert.Zero(t, withheld.Counts(movedAt).Total())
}

func TestWithheldReadinessSkipsAPodOnANodeThatIsNotReady(t *testing.T) {
	w := newReadinessWorld(t, running())
	node := newID(kube.KindNode, "", "n1")
	observeEntity(w.model, node, movedAt, conditionAttrs(nil, "Ready",
		"Unknown", "NodeStatusUnknown", movedAt.Add(-time.Minute)))
	relateEntity(w.model, w.pod, inventory.RunsOn, node)
	assert.Zero(t, w.detectBoth())
}

func TestWithheldReadinessCountsAPodOnAReadyNode(t *testing.T) {
	w := newReadinessWorld(t, running())
	node := newID(kube.KindNode, "", "n1")
	observeEntity(w.model, node, movedAt, conditionAttrs(nil, "Ready",
		"True", "KubeletReady", movedAt.Add(-time.Hour)))
	relateEntity(w.model, w.pod, inventory.RunsOn, node)
	assert.Equal(t, 1, w.detectBoth())
}
