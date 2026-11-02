package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A CronJob starting its own Job is what it does, not a change next to its
// failure. A Job somebody else created, and any other edit, still count.
func TestRecentChangesLeavesOutTheCronJobsOwnScheduledRun(t *testing.T) {
	r := newRig(t, Config{})
	cron := entity(kube.KindCronJob, "stats-rollup")
	run := entity(kube.KindJob, "stats-rollup-292510")
	manual := entity(kube.KindJob, "backfill")
	cm := entity(kube.KindConfigMap, "rollup-config")
	for _, id := range []inventory.EntityID{cron, run, manual, cm} {
		r.observe(id)
	}
	r.relate(run, inventory.OwnedBy, cron)
	r.change(run, 2*time.Minute, inventory.Change{Created: true})
	r.change(manual, 3*time.Minute, inventory.Change{Created: true})
	r.change(cm, 4*time.Minute, dataEdit("bob"))

	got := RecentChanges(r.model, cron, at(10*time.Minute))

	assert.Equal(t, []inventory.EntityID{cm, manual}, entitiesOf(got))
}

// With nothing else changed, the list is empty and the message has no line.
func TestRecentChangesIsEmptyWhenOnlyTheScheduledRunChanged(t *testing.T) {
	r := newRig(t, Config{})
	cron := entity(kube.KindCronJob, "stats-rollup")
	run := entity(kube.KindJob, "stats-rollup-292510")
	r.observe(cron)
	r.observe(run)
	r.relate(run, inventory.OwnedBy, cron)
	r.change(run, 2*time.Minute, inventory.Change{Created: true})

	assert.Empty(t, RecentChanges(r.model, cron, at(10*time.Minute)))
}
