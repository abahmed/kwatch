package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// failedRun is a Job of a CronJob that reads ref and failed, with the
// edit of ref hours before the run started.
func failedRun(
	f *fixture, kind inventory.Kind, lastSuccess time.Time,
) inventory.EntityID {
	ref := inventory.CoreID(kind, "reports", "export-creds")
	cron := inventory.CoreID(kube.KindCronJob, "reports", "nightly")
	job := inventory.CoreID(kube.KindJob, "reports", "nightly-1")
	f.add(ref)
	f.change(ref, -120, "data.token")
	attrs := map[string]inventory.Value{}
	if !lastSuccess.IsZero() {
		attrs[kube.AttrLastSuccess] = inventory.Time(lastSuccess)
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: cron,
		Attributes: attrs})
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: job,
		Attributes: map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(t0.Add(5 * time.Minute))}})
	f.relate(job, inventory.OwnedBy, cron)
	f.relate(job, inventory.References, ref)
	f.fail(job, "Failed", failingH, 6, "")
	return job
}

var runConfigRowCases = []rowCase{
	{row: "run-secret-missing-or-changed",
		want: "secret/reports/export-creds",
		build: func(f *fixture) inventory.EntityID {
			return failedRun(f, kube.KindSecret, time.Time{})
		}},
	{row: "run-configmap-missing-or-changed",
		want: "configmap/reports/export-creds",
		build: func(f *fixture) inventory.EntityID {
			return failedRun(f, kube.KindConfigMap, time.Time{})
		}},
}

// A run that worked after the edit shows the edit harmless.
func TestRunConfigIgnoresAnEditBeforeTheLastSuccess(t *testing.T) {
	f := newFixture(t)
	job := failedRun(f, kube.KindSecret, t0.Add(-time.Hour))

	if c, ok := f.explain().CauseOf(job); ok &&
		c.Row == "run-secret-missing-or-changed" {
		t.Fatalf("blamed an edit older than the last success: %+v", c)
	}
}
