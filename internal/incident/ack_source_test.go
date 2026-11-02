package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestAckRecordsWhenAndWhoSetIt(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	r.change(pod, 30*time.Second, inventory.Change{
		Actor: "kubectl-annotate",
		Fields: []inventory.FieldChange{{
			Path:  "metadata.annotations." + kube.AckAnnotation,
			After: "on it"}},
	})
	setAck(t, r, pod, "on it")
	first := start.Add(time.Minute)
	ds := r.tick(first)
	wantAction(t, ds, Update, ReasonAcknowledged)
	ack := r.only().Ack
	require.NotNil(t, ack)
	assert.Equal(t, first, ack.Since)
	assert.Equal(t, "kubectl-annotate", ack.By)

	// The same note seen again keeps its time.
	wantNone(t, r.tick(start.Add(2*time.Minute)))
	assert.Equal(t, first, r.only().Ack.Since)

	setAck(t, r, pod, "rolling back")
	edited := start.Add(3 * time.Minute)
	wantNone(t, r.tick(edited))
	assert.Equal(t, edited, r.only().Ack.Since, "an edit is a new note")

	clearAck(t, r, pod)
	wantAction(t, r.tick(start.Add(4*time.Minute)), Update,
		ReasonAckRemoved)
	assert.Nil(t, r.only().Ack)
}

func TestAckWithoutAnEditInHistoryNamesNoTool(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	setAck(t, r, pod, "on it")
	r.tick(start.Add(time.Minute))
	ack := r.only().Ack
	require.NotNil(t, ack)
	assert.Empty(t, ack.By)
	assert.Equal(t, start.Add(time.Minute), ack.Since)

	records := r.m.Export()
	assert.Equal(t, ack.Since, records[0].Ack.Since)
}

// The managed fields name the writer of the annotation, which the model
// holds as an attribute next to the ack.
func TestAckByComesFromTheAnnotationWriter(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	observeAttrs(t, r, pod, map[string]inventory.Value{
		kube.AttrAck:   inventory.Text("on it"),
		kube.AttrAckBy: inventory.Text("kubectl-annotate")})
	wantAction(t, r.tick(start.Add(time.Minute)), Update,
		ReasonAcknowledged)
	ack := r.only().Ack
	require.NotNil(t, ack)
	assert.Equal(t, "kubectl-annotate", ack.By)
}
