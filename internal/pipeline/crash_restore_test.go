package pipeline

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/storage"
	"github.com/abahmed/kwatch/internal/storage/crashtest"
)

// What a restart finds after kwatch was killed in the middle of the
// pipeline's own writes: incident records, the change store and the
// digest state. The child writes through the real incident store and is
// killed with SIGKILL; the parent opens the file as a restart does.

var crashBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// TestPipelineCrashChild is the child of the tests below.
func TestPipelineCrashChild(t *testing.T) {
	crashtest.Serve(t, func(path, mode string, start int,
		ack func(int), _ func(string)) {
		s, err := storage.Open(path, storage.Options{Now: time.Now})
		if err != nil {
			panic(err)
		}
		if _, err := s.Claim(); err != nil {
			panic(err)
		}
		store := NewIncidentStore(s)
		for i := start; ; i++ {
			if err := crashWrite(store, mode, i); err != nil {
				panic(err)
			}
			ack(i)
		}
	})
}

func crashID(i int) string { return fmt.Sprintf("inc-%05d", i) }

func crashRecord(i int) incident.Record {
	return incident.Record{
		ID: crashID(i), Root: crashEntity(i), Tier: incident.Notify,
		State: incident.Open, Opened: crashBase, Announced: crashBase,
	}
}

func crashEntity(i int) inventory.EntityID {
	return inventory.CoreID(kube.KindDeployment, "shop",
		fmt.Sprintf("api-%d", i))
}

// crashWrite is write i of the mode: the open incidents 0..i, one more
// change, or the digest that lists incidents 0..i.
func crashWrite(store IncidentStore, mode string, i int) error {
	switch mode {
	case "incidents":
		records := make([]incident.Record, 0, i+1)
		for n := 0; n <= i; n++ {
			records = append(records, crashRecord(n))
		}
		return store.SaveIncidents(records)
	case "changes":
		return store.(changeStore).AppendChanges([]changeRecord{{
			Change: inventory.Change{Entity: crashEntity(i),
				At: crashBase.Add(time.Duration(i) * time.Second)},
		}})
	default: // "digest"
		var state announce.DigestState
		for n := 0; n <= i; n++ {
			state.Listed = append(state.Listed, crashID(n))
		}
		return store.SaveStartup(announce.StartupState{
			Complete: true, Digest: state})
	}
}

// crashRestore opens the file as a restart does and returns how many
// writes it holds. It fails when a write below acked is missing or the
// state is not one a write left.
func crashRestore(t *testing.T, path, mode string, acked int) int {
	t.Helper()
	s, err := storage.Open(path, storage.Options{Now: time.Now})
	require.NoError(t, err, "the file does not open")
	defer func() { require.NoError(t, s.Close()) }()
	_, err = s.Claim()
	require.NoError(t, err)
	store := NewIncidentStore(s)

	var held int
	switch mode {
	case "incidents":
		records, err := store.LoadIncidents()
		require.NoError(t, err)
		// The manager restores every open incident, so none is lost.
		m := incident.NewManager(incident.Config{}, nil)
		m.Restore(records, crashBase)
		exported := m.Export()
		held = len(exported)
		for _, r := range exported {
			assert.Equal(t, incident.Open, r.State, r.ID)
		}
		for i := 0; i < held; i++ {
			require.Contains(t, idsOf(exported), crashID(i),
				"incident %d missing between others", i)
		}
	case "changes":
		got, err := store.(changeStore).LoadChanges(time.Time{}, 0)
		require.NoError(t, err)
		held = len(got)
		for i, c := range got {
			assert.Equal(t, crashEntity(i), c.Change.Entity)
		}
	default:
		state, found, err := store.LoadStartup()
		require.NoError(t, err)
		if found {
			held = len(state.Digest.Listed)
			for i, id := range state.Digest.Listed {
				require.Equal(t, crashID(i), id, "digest state is torn")
			}
		}
	}
	require.GreaterOrEqual(t, held, acked+1,
		"a write that was acked is missing")
	return held
}

func idsOf(records []incident.Record) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r.ID)
	}
	return out
}

func crashRounds(t *testing.T, mode string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	held := 0
	for round := 0; round < 6; round++ {
		acked := crashtest.Crash(t, crashtest.Options{
			Test: "TestPipelineCrashChild", Mode: mode, Path: path,
			Start:    held,
			AfterAck: held + 5 + 3*round,
			Delay:    time.Duration(round) * 3 * time.Millisecond,
		})
		held = crashRestore(t, path, mode, acked)
	}
}

// Open incidents survive a kill during the incident record write.
func TestCrashDuringIncidentSaveKeepsOpenIncidents(t *testing.T) {
	crashRounds(t, "incidents")
}

// The change store holds a prefix of what was flushed after a kill.
func TestCrashDuringChangeFlushKeepsFlushedChanges(t *testing.T) {
	crashRounds(t, "changes")
}

// The digest state is the last one written whole, never a mix.
func TestCrashDuringDigestStateWriteLeavesAWholeState(t *testing.T) {
	crashRounds(t, "digest")
}
