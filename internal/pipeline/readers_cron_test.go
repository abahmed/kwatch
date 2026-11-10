package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A failed run is history once its CronJob records a later success, so
// an observed CronJob has its Jobs, their pods and containers judged
// again: nothing else touches them when the status moves.
func TestEngineMarksTheRunsOfAnUpdatedCronJob(t *testing.T) {
	e := solveInputEngine(t)
	cron := inventory.CoreID(kube.KindCronJob, "shop", "rollup")
	job := inventory.CoreID(kube.KindJob, "shop", "rollup-1")
	pod := inventory.CoreID(kube.KindPod, "shop", "rollup-1-a")
	container := inventory.CoreID(kube.KindContainer, "shop",
		"rollup-1-a/task")
	e.apply([]inventory.Observation{
		seen(cron, time.Now(), nil), seen(job, time.Now(), nil),
		seen(pod, time.Now(), nil), seen(container, time.Now(), nil),
		link(job, inventory.OwnedBy, cron),
		link(pod, inventory.OwnedBy, job),
		link(container, inventory.PartOf, pod),
	})
	dirty := e.apply([]inventory.Observation{seen(cron, time.Now(),
		map[string]inventory.Value{
			kube.AttrLastSuccess: inventory.Time(time.Now()),
		})})
	for _, want := range []inventory.EntityID{job, pod, container} {
		if !containsEntity(dirty, want) {
			t.Fatalf("dirty = %v, want %s", dirty, want)
		}
	}
}
