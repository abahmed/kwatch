package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The node controller removes a node's taints itself when the node is
// ready again. That is the recovery, not a change someone made, and it
// is not credited as the fix: before, every node that came back was
// "fixed by a change".
func TestNodeTaintChangeIsNotCreditedAsTheFix(t *testing.T) {
	r := newRig(t, Config{})
	node := lostNode("n1")
	r.observe(node.Entity)
	r.change(node.Entity, 3*time.Minute, inventory.Change{
		Actor: "kube-controller-manager",
		Fields: []inventory.FieldChange{{Path: "spec.taints",
			Before: "node.kubernetes.io/unreachable:NoExecute"}}})

	failOnce(r, node, 0, 4*time.Minute)

	got := r.of(node.Entity)
	assert.Equal(t, FixNone, got.Fix)
	assert.Nil(t, got.FixedBy)
}

// A crashing pod that someone deletes is replaced by its controller.
// The pod records (one deleted, one created) are not a change that
// fixed anything.
func TestReplacedPodIsNotCreditedAsTheFix(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	r.change(web.Entity, 3*time.Minute, inventory.Change{
		Actor: "kubectl", Deleted: true})
	next := entity(kube.KindPod, "web-next")
	r.observe(next)
	r.relate(web.Entity, inventory.References, next)
	r.change(next, 3*time.Minute+time.Second, inventory.Change{
		Actor: "kube-controller-manager", Created: true})

	failOnce(r, web, 0, 4*time.Minute)

	got := r.of(web.Entity)
	assert.Equal(t, FixNone, got.Fix)
	assert.Nil(t, got.FixedBy)
}

func TestNotAFix(t *testing.T) {
	pod, node := entity(kube.KindPod, "web"), nodeID()
	edit := []inventory.FieldChange{{Path: "spec.template"}}
	taint := []inventory.FieldChange{{Path: "spec.taints"}}
	assert.True(t, notAFix(inventory.Change{Entity: pod, Created: true}))
	assert.True(t, notAFix(inventory.Change{Entity: pod, Deleted: true}))
	assert.True(t, notAFix(inventory.Change{Entity: node, Fields: taint}))
	assert.False(t, notAFix(inventory.Change{Entity: pod, Fields: edit}))
	assert.False(t, notAFix(inventory.Change{Entity: node, Fields: edit}))
	assert.False(t, notAFix(inventory.Change{
		Entity: entity(kube.KindConfigMap, "cfg"), Created: true}))
}

// The next scheduled Job of a failing CronJob runs fine: the CronJob
// made it, so it is not someone's fix. An edit of the CronJob still is.
func TestControllerMadeJobIsNotCreditedAsTheFix(t *testing.T) {
	r := newRig(t, Config{})
	cron := sig(entity(kube.KindCronJob, "rollup"), "JobFailed",
		detection.Warning)
	r.observe(cron.Entity)
	next := entity(kube.KindJob, "rollup-29001")
	r.observe(next)
	r.relate(next, inventory.OwnedBy, cron.Entity)
	r.relate(cron.Entity, inventory.References, next)
	r.change(next, 3*time.Minute, inventory.Change{
		Actor: "cronjob-controller", Created: true})

	failOnce(r, cron, 0, 4*time.Minute)

	got := r.of(cron.Entity)
	assert.Equal(t, FixNone, got.Fix)
	assert.Nil(t, got.FixedBy)
}

func TestControllerChild(t *testing.T) {
	cron := entity(kube.KindCronJob, "rollup")
	deploy := entity(kube.KindDeployment, "api")
	job := inventory.Change{Entity: entity(kube.KindJob, "rollup-1"),
		Created: true}
	rs := inventory.Change{Entity: entity(kube.KindReplicaSet, "api-7f"),
		Deleted: true}
	other := inventory.Change{Entity: entity(kube.KindJob, "report-1"),
		Created: true}
	edit := inventory.Change{Entity: entity(kube.KindJob, "rollup-1"),
		Fields: []inventory.FieldChange{{Path: "spec.suspend"}}}
	assert.True(t, controllerChild(nil, cron, job))
	assert.True(t, controllerChild(nil, deploy, rs))
	assert.False(t, controllerChild(nil, cron, other))
	assert.False(t, controllerChild(nil, cron, edit))
	assert.False(t, controllerChild(nil, deploy, job))
}
