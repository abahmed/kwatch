package pushover

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func emergencyPushover(rec *providertest.Recorder) *Pushover {
	c := NewPushover(map[string]interface{}{
		"token": "tok", "user": "u", "priority": 2,
		"retry": 60, "expire": 3600,
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/1/messages.json"
	return c
}

func TestResolveCancelsTheEmergencyReceipt(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"status":1,"receipt":"rcpt1"}`))
	}
	c := emergencyPushover(rec)
	ctx := context.Background()

	assert.NoError(t, c.SendIncident(ctx, providertest.Announce()))
	assert.NoError(t, c.SendIncident(ctx, providertest.Update()))
	assert.NoError(t, c.SendIncident(ctx, providertest.Resolve()))

	requests := rec.Requests()
	assert.Len(t, requests, 4, "announce, update, cancel, resolve")
	update, _ := url.ParseQuery(string(requests[1].Body))
	assert.False(t, update.Has("priority"), "update must not alarm")
	assert.Equal(t, "/1/receipts/rcpt1/cancel.json", requests[2].Path)
	assert.Equal(t, "/1/messages.json", requests[3].Path)
	cancel, _ := url.ParseQuery(string(requests[2].Body))
	assert.Equal(t, "tok", cancel.Get("token"))

	// The receipt is gone, so a second resolve does not cancel again.
	assert.NoError(t, c.SendIncident(ctx, providertest.Resolve()))
	assert.Len(t, rec.Requests(), 5)
}

func TestSummariesAndNoticesAreNormalPriority(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := emergencyPushover(rec)
	ctx := context.Background()

	assert.NoError(t, c.SendIncident(ctx, providertest.Summary()))
	assert.NoError(t, c.SendMessage(ctx, "kwatch started"))
	for _, request := range rec.Requests() {
		form, _ := url.ParseQuery(string(request.Body))
		assert.False(t, form.Has("priority"), form.Encode())
	}
}

func TestNonPageAnnounceIsNotAnEmergency(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := emergencyPushover(rec)
	m := providertest.Announce()
	m.Status = 2 // StatusWarning
	assert.NoError(t, c.SendIncident(context.Background(), m))
	form, _ := url.ParseQuery(string(rec.Last(t).Body))
	assert.Equal(t, "1", form.Get("priority"))
	assert.False(t, form.Has("retry"))
}

func TestReceiptsSurviveRestart(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := emergencyPushover(rec)
	c.RestoreThreads(map[string]string{"p-42": "old"})
	assert.Equal(t, map[string]string{"p-42": "old"}, c.SnapshotThreads())
	assert.NoError(t, c.SendIncident(
		context.Background(), providertest.Resolve()))
	assert.Equal(t, "/1/receipts/old/cancel.json",
		rec.Requests()[0].Path)
}

func TestFailedCancelSendsNoResolvedPushUntilItSucceeds(t *testing.T) {
	rec := providertest.NewRecorder(t)
	failCancel := false
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if failCancel && strings.Contains(r.Path, "/cancel.json") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"status":1,"receipt":"rcpt1"}`))
	}
	c := emergencyPushover(rec)
	ctx := context.Background()
	assert.NoError(t, c.SendIncident(ctx, providertest.Announce()))

	failCancel = true
	assert.Error(t, c.SendIncident(ctx, providertest.Resolve()))
	assert.Len(t, rec.Requests(), 2, "announce and the failed cancel only")

	failCancel = false
	assert.NoError(t, c.SendIncident(ctx, providertest.Resolve()))
	assert.Len(t, rec.Requests(), 4, "cancel again, then one resolve push")
}
