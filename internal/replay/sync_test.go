package replay_test

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

// Signalling the sync on a cold start collects the announcements of the
// startup window into one startup summary.
func TestRunSignalsSourcesSyncedForTheStartupSummary(t *testing.T) {
	log := readBytes(t, recordBadRollout(t))
	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{SyncAt: rolloutStart.Add(70 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	summaries := 0
	for _, d := range result.Decisions {
		if d.Reason == "startup summary" {
			summaries++
		}
	}
	if summaries != 1 {
		t.Fatalf("decisions = %q, want one startup summary",
			actions(result))
	}
}

func TestRunWithoutSyncSendsNoStartupSummary(t *testing.T) {
	result := replayFile(t, badRolloutLog)
	for _, d := range result.Decisions {
		if d.Reason == "startup summary" {
			t.Fatal("startup summary without a sync signal")
		}
	}
}

// A sync after the last entry extends the replay so the signal is sent.
func TestRunExtendsTheEndToALateSync(t *testing.T) {
	log := replay.Log{Start: rolloutStart}
	syncAt := rolloutStart.Add(time.Hour)
	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{SyncAt: syncAt, Tail: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if want := syncAt.Add(time.Minute); !result.End.Equal(want) {
		t.Fatalf("end %s, want %s", result.End, want)
	}
}

func TestRunRecordsDeliveryTimes(t *testing.T) {
	log := readBytes(t, recordBadRollout(t))
	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Times) != len(result.Messages) {
		t.Fatalf("%d times for %d messages", len(result.Times),
			len(result.Messages))
	}
	for i, at := range result.Times {
		if at.Before(log.Start) || at.After(result.End) ||
			(i > 0 && at.Before(result.Times[i-1])) {
			t.Fatalf("time %d = %s is out of order", i, at)
		}
	}
}
