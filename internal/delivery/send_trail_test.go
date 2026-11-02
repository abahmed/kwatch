package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func trailJob(key string) deliverJob {
	return deliverJob{kind: jobIncident,
		incident: &notification.Message{Key: key, Opens: true}}
}

func TestSendTrailRecordsEachProviderOutcome(t *testing.T) {
	provider := &errorRecorderProvider{name: "slack"}
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{provider: provider,
		retry: retryConfig{maxAttempts: 1, delay: time.Millisecond}}})
	entry := managerEntries(am)[0]
	ctx := context.Background()

	_ = am.dispatch(ctx, &entry, trailJob("inc-1"), deliverOpts{})
	provider.err = transport.Permanent(errors.New("bad token"))
	_ = am.dispatch(ctx, &entry, trailJob("inc-1"), deliverOpts{})
	provider.err = errors.New("timeout https://hooks.example/secret")
	_ = am.dispatch(ctx, &entry, trailJob("inc-1"), deliverOpts{})

	got := am.SendTrail("inc-1")
	if len(got) != 3 {
		t.Fatalf("trail = %+v", got)
	}
	want := []string{"ok", "failed", "retrying"}
	for i, a := range got {
		if a.Result != want[i] || a.Provider != "slack" ||
			a.Kind != "incident" || a.Placement != "root" {
			t.Errorf("attempt %d = %+v", i, a)
		}
	}
	if got[2].Error == "" || strings.Contains(got[2].Error, "secret") {
		t.Errorf("error not scrubbed: %q", got[2].Error)
	}
	if am.SendTrail("other") != nil {
		t.Error("another incident has a trail")
	}
}

func TestSendTrailRecordsCarriedMessages(t *testing.T) {
	am := newTestManager()
	am.NotifyIncident(notification.Message{Key: "inc-2",
		Carrier: "digest"})
	got := am.SendTrail("inc-2")
	if len(got) != 1 || got[0].Result != "carried" ||
		got[0].Carrier != "digest" || got[0].Provider != "" {
		t.Errorf("trail = %+v", got)
	}
}

func TestSendTrailIsBounded(t *testing.T) {
	var trail sendTrail
	for i := range maxTrailAttempts + 3 {
		trail.add("a", SendAttempt{Retries: i})
	}
	got := trail.of("a")
	if len(got) != maxTrailAttempts || got[0].Retries != 3 {
		t.Errorf("kept %d, first %d", len(got), got[0].Retries)
	}
	for i := range maxTrailKeys + 1 {
		trail.add(fmt.Sprint("k", i), SendAttempt{})
	}
	if trail.of("a") != nil || len(trail.byKey) != maxTrailKeys {
		t.Errorf("oldest incident kept; %d keys", len(trail.byKey))
	}
}

// The first accepted attempt is kept when newer attempts fill the
// bound: it is when people were first told.
func TestSendTrailKeepsFirstAcceptedAttempt(t *testing.T) {
	var trail sendTrail
	trail.add("a", SendAttempt{Result: "retrying"})
	trail.add("a", SendAttempt{Result: resultOK, Retries: 99})
	for range maxTrailAttempts + 3 {
		trail.add("a", SendAttempt{Result: "retrying"})
	}
	got := trail.of("a")
	if len(got) != maxTrailAttempts {
		t.Fatalf("kept %d attempts", len(got))
	}
	if got[0].Result != resultOK || got[0].Retries != 99 {
		t.Errorf("first accepted attempt lost: %+v", got[0])
	}
	if got[len(got)-1].Result != "retrying" {
		t.Errorf("newest attempt lost: %+v", got[len(got)-1])
	}
}

// The bound forgets the incident told least recently, not the one
// first told longest ago.
func TestSendTrailForgetsLeastRecentlyTold(t *testing.T) {
	var trail sendTrail
	trail.add("busy", SendAttempt{})
	for i := range maxTrailKeys - 1 {
		trail.add(fmt.Sprint("k", i), SendAttempt{})
	}
	trail.add("busy", SendAttempt{})
	trail.add("new", SendAttempt{})
	if trail.of("busy") == nil {
		t.Error("an incident told just now was forgotten")
	}
	if trail.of("k0") != nil {
		t.Error("the least recently told incident was kept")
	}
	if len(trail.order) != maxTrailKeys {
		t.Errorf("order holds %d keys", len(trail.order))
	}
}

// Summaries, roll-ups and digests take no place in the trail; the
// incidents a startup summary or roll-up carried get a carried attempt.
func TestSendTrailSkipsSummariesAndNotesBulkCarriers(t *testing.T) {
	am := newTestManager()
	for _, key := range []string{notification.SummaryKeyPrefix + "1",
		notification.RollupKeyPrefix + "2",
		notification.DigestKeyPrefix + "3"} {
		am.noteSent(sendRecord{provider: "slack"}, trailJob(key))
		am.noteUnsent(trailJob(key), "not routed", "")
		if got := am.SendTrail(key); got != nil {
			t.Errorf("summary %s has a trail: %+v", key, got)
		}
	}
	for _, carrier := range []string{"startup summary", "roll-up"} {
		am.NotifyIncident(notification.Message{Key: "inc-" + carrier,
			Carrier: carrier})
		got := am.SendTrail("inc-" + carrier)
		if len(got) != 1 || got[0].Result != "carried" ||
			got[0].Carrier != carrier {
			t.Errorf("%s: trail = %+v", carrier, got)
		}
	}
}
