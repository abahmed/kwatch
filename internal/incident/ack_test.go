package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// setAck puts the ack attribute on id, as the translator does for an
// object that carries the annotation.
func setAck(t *testing.T, r *rig, id inventory.EntityID, note string) {
	t.Helper()
	observeAttrs(t, r, id, map[string]inventory.Value{
		kube.AttrAck: inventory.Text(note)})
}

// clearAck removes the attribute, as an observation without it does.
func clearAck(t *testing.T, r *rig, id inventory.EntityID) {
	t.Helper()
	observeAttrs(t, r, id, nil)
}

func observeAttrs(
	t *testing.T, r *rig, id inventory.EntityID,
	attrs map[string]inventory.Value,
) {
	t.Helper()
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: t0, Entity: id, Attributes: attrs,
	})
	require.NoError(t, err)
}

func TestAckPostsOneUpdateAndStopsReminders(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")

	setAck(t, r, pod, "looking into it")
	ds := r.tick(start.Add(time.Minute))
	wantAction(t, ds, Update, ReasonAcknowledged)
	require.NotNil(t, ds[0].Incident.Ack)
	assert.Equal(t, pod, ds[0].Incident.Ack.On)
	assert.Equal(t, "looking into it", ds[0].Incident.Ack.Note)

	wantNone(t, r.tick(start.Add(2*time.Minute)))
	wantNone(t, r.tick(start.Add(RemindEvery)))
	wantNone(t, r.tick(start.Add(3*RemindEvery)))
}

func TestAckOnAMemberOfTheIncident(t *testing.T) {
	r := newRig(t, Config{})
	root, member := entity(kube.KindNode, "n1"), entity(kube.KindPod, "web")
	r.cause(member, root, "node n1 is down")
	r.raise(at(0), podSig("web"))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	require.Equal(t, root, r.only().Root)

	setAck(t, r, member, "")
	ds := r.tick(at(DefaultSettle + time.Minute))

	wantAction(t, ds, Update, ReasonAcknowledged)
	assert.Equal(t, member, ds[0].Incident.Ack.On)
	assert.Empty(t, ds[0].Incident.Ack.Note)
}

func TestAckRemovedResumesRemindersFromThen(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	setAck(t, r, pod, "on it")
	wantAction(t, r.tick(start.Add(time.Minute)), Update, ReasonAcknowledged)

	removed := start.Add(RemindEvery + time.Hour)
	clearAck(t, r, pod)
	ds := r.tick(removed)
	wantAction(t, ds, Update, ReasonAckRemoved)
	assert.Nil(t, ds[0].Incident.Ack)

	wantNone(t, r.tick(removed.Add(RemindEvery-time.Second)))
	wantAction(t, r.tick(removed.Add(RemindEvery)), Update, ReasonReminder)
}

func TestAckPresentAtAnnouncementIsMentionedNotPosted(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	setAck(t, r, pod, "known issue")
	r.raise(at(0), podSig("web"))

	ds := r.tick(at(DefaultSettle))

	wantAction(t, ds, Announce, "settled")
	require.NotNil(t, ds[0].Incident.Ack)
	assert.True(t, ds[0].Incident.Ack.AtAnnounce)
	wantNone(t, r.tick(at(DefaultSettle+RemindEvery)))
}

func TestAckStaysUntilTheAnnotationIsGone(t *testing.T) {
	r, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	setAck(t, r, pod, "first note")
	wantAction(t, r.tick(start.Add(time.Minute)), Update, ReasonAcknowledged)

	setAck(t, r, pod, "second note")
	wantNone(t, r.tick(start.Add(2*time.Minute)))
	assert.Equal(t, "second note", r.only().Ack.Note, "kept, not told")
}

func TestAckSurvivesARestore(t *testing.T) {
	src, start := announcedRig(t)
	pod := entity(kube.KindPod, "web")
	setAck(t, src, pod, "on it")
	wantAction(t, src.tick(start.Add(time.Minute)), Update,
		ReasonAcknowledged)

	dst := newRig(t, Config{})
	dst.m.Restore(src.m.Export(), time.Time{})

	got := dst.only().Ack
	require.NotNil(t, got)
	assert.Equal(t, "on it", got.Note)
}

func TestAckedIncidentStillSendsItsResolve(t *testing.T) {
	r, start := announcedRig(t)
	setAck(t, r, entity(kube.KindPod, "web"), "on it")
	wantAction(t, r.tick(start.Add(time.Minute)), Update, ReasonAcknowledged)

	cleared := start.Add(2 * time.Minute)
	r.clear(cleared, podSig("web"))
	var ds []Decision
	for now := cleared; now.Before(cleared.Add(time.Hour)); now =
		now.Add(10 * time.Second) {
		ds = append(ds, r.tick(now)...)
	}

	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

func nodeID() inventory.EntityID {
	return inventory.CoreID(kube.KindNode, "", "n1")
}

func TestAckStopsThePageReminder(t *testing.T) {
	r, start := pagedRig(t)
	setAck(t, r, nodeID(), "replacing the node")
	ds := r.tick(start.Add(time.Minute))
	wantAction(t, ds, Update, ReasonAcknowledged)
	assert.Equal(t, Page, ds[0].Incident.Tier, "still a page, told once")

	wantNone(t, r.tick(start.Add(PageRemindAfter)))
	wantNone(t, r.tick(start.Add(RemindEvery)))
}

func TestAckStillPresentAppliesToAReopenInsideTheRepageWindow(t *testing.T) {
	r, _ := pagedRig(t)
	setAck(t, r, nodeID(), "replacing the node")
	wantAction(t, r.tick(r.only().Announced.Add(time.Minute)), Update,
		ReasonAcknowledged)
	resolved := resolvePage(t, r, 5*time.Minute)

	d, raiseAt := failAgain(t, r, resolved, 10*time.Minute)

	assert.Equal(t, ReasonFailingAgain, d.Reason, "the thread hears it is back")
	require.NotNil(t, d.Incident.Ack, "same incident, same acknowledgement")
	start := at(raiseAt + DefaultSettle)
	wantNone(t, r.tick(start.Add(PageRemindAfter)))
	wantNone(t, r.tick(start.Add(RemindEvery)))
}

func TestNewIncidentAfterTheRepageWindowIsNotPreAcked(t *testing.T) {
	r, _ := pagedRig(t)
	setAck(t, r, nodeID(), "replacing the node")
	wantAction(t, r.tick(r.only().Announced.Add(time.Minute)), Update,
		ReasonAcknowledged)
	resolved := resolvePage(t, r, 5*time.Minute)
	clearAck(t, r, nodeID())

	raiseAt := resolved + RepageWindow + time.Minute
	r.raise(at(raiseAt), nodeDown())
	ds := tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)

	require.NotEmpty(t, ds)
	assert.Equal(t, Announce, ds[0].Action, "notifies normally")
	assert.Nil(t, ds[0].Incident.Ack)
}

func TestNewIncidentWithTheAnnotationStillThereSaysSo(t *testing.T) {
	r, _ := pagedRig(t)
	setAck(t, r, nodeID(), "replacing the node")
	wantAction(t, r.tick(r.only().Announced.Add(time.Minute)), Update,
		ReasonAcknowledged)
	resolved := resolvePage(t, r, 5*time.Minute)

	raiseAt := resolved + RepageWindow + time.Minute
	r.raise(at(raiseAt), nodeDown())
	ds := tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)

	require.NotEmpty(t, ds)
	assert.Equal(t, Announce, ds[0].Action)
	require.NotNil(t, ds[0].Incident.Ack)
	assert.True(t, ds[0].Incident.Ack.AtAnnounce)
}

func TestRestoredAckIsNotRemovedWhileTheModelIsRebuilt(t *testing.T) {
	src, start := announcedRig(t)
	setAck(t, src, entity(kube.KindPod, "web"), "on it")
	wantAction(t, src.tick(start.Add(time.Minute)), Update,
		ReasonAcknowledged)

	dst := newRig(t, Config{})
	grace := start.Add(5 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(start.Add(2*time.Minute), podSig("web"))

	wantNone(t, dst.tick(start.Add(3*time.Minute)))
	assert.NotNil(t, dst.only().Ack, "the annotation is not seen yet")
}
