package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func idleHook() inventory.EntityID {
	return inventory.CoreID(kube.KindValidatingHook, "", "leftover")
}

// deadBackend is the finding of a fail-closed webhook whose backend is
// gone, at the severity the detector gives it.
func deadBackend(severity detection.Severity) detection.Finding {
	return sig(idleHook(), reasons.WebhookBackendNotFound, severity)
}

// A dead backend nobody was refused by is announced once, as a
// notification, and never pages.
func TestIdleWebhookNotifiesAndDoesNotPage(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, deadBackend(detection.Warning))
	p := r.only()
	assert.Equal(t, Notify, p.Tier)
	assert.Empty(t, pageRuleOf(&p), "no page rule matches it")
}

// After its announcement it waits for the digest without a word to its
// thread, and without the "still open" reminders of a digest incident.
func TestIdleWebhookFallsToDigestQuietly(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, deadBackend(detection.Warning))

	start := r.only().Announced
	for _, d := range r.tick(start.Add(time.Hour)) {
		t.Errorf("unexpected decision %v %v", d.Action, d.Reason)
	}
	p := r.only()
	assert.Equal(t, Digest, p.Tier)
	assert.True(t, p.Delivery.Demoted(), "the digest lists it as ongoing")

	for _, wait := range []time.Duration{RemindEvery, 8 * 24 * time.Hour} {
		wantNone(t, r.tick(start.Add(wait)))
	}
	assert.Equal(t, Digest, r.only().Tier, "no flip back to notify")
}

// The first refused request makes it the outage it always could be: the
// finding turns critical and the incident pages.
func TestIdleWebhookPagesWhenARequestIsRefused(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, deadBackend(detection.Warning))
	r.tick(r.only().Announced.Add(time.Hour))
	require.Equal(t, Digest, r.only().Tier)

	now := at(3 * time.Hour)
	r.apply(now, detection.Changed, deadBackend(detection.Critical))
	ds := r.tick(now)
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Page, ds[0].Incident.Tier)
}

// A page restored from before webhooks needed a refused request has
// nothing to stand on once its findings return as warnings: it drops.
func TestIdleWebhookRestoredPageDrops(t *testing.T) {
	src := newRig(t, Config{})
	src.raise(at(0), deadBackend(detection.Critical))
	wantAction(t, src.tick(at(DefaultPageSettle)), Announce, "settled")
	require.Equal(t, Page, src.only().Tier)

	recs := src.m.Export()
	recs[0].RejectionSeenAt = time.Time{} // a record from before the field
	dst := newRig(t, Config{})
	grace := at(10 * time.Minute)
	dst.m.Restore(recs, grace)
	dst.raise(at(time.Minute), deadBackend(detection.Warning))
	assert.Equal(t, Notify, dst.only().Tier, "the page was not earned")
}

// A page that saw a refusal in this process stays a page when the
// refusals stop being observed: nothing is un-paged on a quiet minute.
func TestIdleWebhookPageThatWasEarnedStays(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), deadBackend(detection.Critical))
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, "settled")
	require.Equal(t, Page, r.only().Tier)

	r.apply(at(time.Hour), detection.Changed, deadBackend(detection.Warning))
	assert.Equal(t, Page, r.only().Tier)
}

// Findings of other failures keep an incident from being idle.
func TestIdleWebhookNeedsOnlyWebhookFindings(t *testing.T) {
	hook := deadBackend(detection.Warning)
	other := sig(entity(kube.KindPod, "web"), reasons.CrashLoopBackOff,
		detection.Warning)
	cases := map[string]struct {
		members []detection.Finding
		want    bool
	}{
		"webhook alone": {[]detection.Finding{hook}, true},
		"critical webhook": {[]detection.Finding{
			deadBackend(detection.Critical)}, false},
		"with a crash": {[]detection.Finding{hook, other}, false},
		"no webhook":   {[]detection.Finding{other}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := &Incident{Members: map[detection.Key]detection.Finding{}}
			for _, s := range c.members {
				p.Members[s.Key()] = s
			}
			assert.Equal(t, c.want, idleWebhook(p))
		})
	}
	blocked := &Incident{admissionBlocked: true,
		Members: map[detection.Key]detection.Finding{hook.Key(): hook}}
	assert.False(t, idleWebhook(blocked), "blocking creates is not idle")
}

// A page that saw a refusal keeps it across a restart: the fact is part
// of the record, so a restored page whose findings return as warnings
// stays a page, as it would in the process that earned it.
func TestIdleWebhookRestoredPageThatSawRejectionStays(t *testing.T) {
	src := newRig(t, Config{})
	src.raise(at(0), deadBackend(detection.Critical))
	wantAction(t, src.tick(at(DefaultPageSettle)), Announce, "settled")
	require.Equal(t, Page, src.only().Tier)
	recs := src.m.Export()
	require.Len(t, recs, 1)
	assert.False(t, recs[0].RejectionSeenAt.IsZero())

	dst := newRig(t, Config{})
	dst.m.Restore(recs, at(10*time.Minute))
	dst.raise(at(time.Minute), deadBackend(detection.Warning))
	assert.Equal(t, Page, dst.only().Tier,
		"the refusal was seen before the restart")
}

// An incident that never saw a refusal records none, and a record
// without the field restores as it always did.
func TestIdleWebhookNeverRejectedRecordsNothing(t *testing.T) {
	src := newRig(t, Config{})
	announced(t, src, deadBackend(detection.Warning))
	recs := src.m.Export()
	require.Len(t, recs, 1)
	assert.True(t, recs[0].RejectionSeenAt.IsZero())
}
