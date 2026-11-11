package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// refusedJob is a Job whose first pod create was refused at t0.
func refusedJob() (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	job := newID(kube.KindJob, "shop", "migrate")
	put(m, job, t0, nil)
	warn(m, job, t0, "FailedCreate", "Error creating: Internal error "+
		"occurred: failed calling webhook: context deadline exceeded")
	return m, job
}

func jobPod(m *inventory.Model, job inventory.EntityID, created time.Time) {
	pod := newID(kube.KindPod, job.Namespace, job.Name+"-x")
	put(m, pod, created, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
	})
	link(m, pod, inventory.OwnedBy, job)
}

// One refused create that the controller's retry got past is over: the
// Job has a pod created after the refusal, or completed.
func TestJobFailedCreateOvercome(t *testing.T) {
	now := t0.Add(time.Minute)
	cases := map[string]struct {
		arrange func(*inventory.Model, inventory.EntityID)
		quiet   bool
	}{
		"no pod yet": {func(*inventory.Model, inventory.EntityID) {},
			false},
		"only a pod from before the refusal": {
			func(m *inventory.Model, job inventory.EntityID) {
				jobPod(m, job, t0.Add(-time.Minute))
			}, false},
		"a pod created after the refusal": {
			func(m *inventory.Model, job inventory.EntityID) {
				jobPod(m, job, t0.Add(10*time.Second))
			}, true},
		"the job completed": {
			func(m *inventory.Model, job inventory.EntityID) {
				setCondition(m, job, "Complete", "True", "", "", now)
			}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, job := refusedJob()
			tc.arrange(m, job)
			got := evaluate(Event{}, m, now, job, nil).Findings
			assert.Equal(t, tc.quiet, len(got) == 0, "%v", got)
		})
	}
}

// The rule is about Jobs: a ReplicaSet that cannot create pods keeps
// its finding while it is short, whatever pods it has.
func TestFailedCreateOnOtherKindsIsKept(t *testing.T) {
	m := newTestModel()
	rs := newID(kube.KindReplicaSet, "shop", "web-1")
	put(m, rs, t0, nil)
	warn(m, rs, t0, "FailedCreate", "Error creating: pods is forbidden")
	jobPod(m, rs, t0.Add(10*time.Second))
	got := evaluate(Event{}, m, t0.Add(time.Minute), rs, nil).Findings
	assert.NotEmpty(t, got)
}
