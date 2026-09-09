package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// A controller writing status is not a change anyone made. Recorded, it named
// the incident's own object as "a related resource that changed 0s ago".
func TestRecordChangeUpdateSkipsStatusOnlyWrites(t *testing.T) {
	tracker := kwcontext.NewChangeTracker(10)
	c := &Controller{tracker: tracker}
	replicas := int32(2)
	before := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}

	statusOnly := before.DeepCopy()
	statusOnly.Status.ReadyReplicas = 2
	c.recordChangeUpdate("deployment", before, statusOnly)
	assert.Empty(t, tracker.Snapshot(), "a status write is not a change")

	more := int32(5)
	specChange := before.DeepCopy()
	specChange.Spec.Replicas = &more
	c.recordChangeUpdate("deployment", before, specChange)
	if assert.Len(t, tracker.Snapshot(), 1) {
		assert.Equal(t, "deployment", tracker.Snapshot()[0].Resource)
	}
}

// A node lease renews every ten seconds; that heartbeat is never a change.
func TestRecordChangeUpdateIgnoresLeaseRenewals(t *testing.T) {
	tracker := kwcontext.NewChangeTracker(10)
	c := &Controller{tracker: tracker}
	before := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n1", Namespace: "kube-node-lease",
		},
	}
	renewed := before.DeepCopy()
	now := metav1.NowMicro()
	renewed.Spec.RenewTime = &now

	c.recordChangeUpdate("lease", before, renewed)
	assert.Empty(t, tracker.Snapshot())

	c.recordChange(kwcontext.ChangeCreate, "lease", before)
	assert.Len(t, tracker.Snapshot(), 1, "creation is still recorded")
}
