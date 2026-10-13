package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/abahmed/kwatch/internal/storage/crashtest"
)

// What a crash leaves behind. The child process writes through the real
// store and is killed with SIGKILL at several points; the parent then
// opens the file the way a restart does. Each write the child acked must
// be there, and a batch is there whole or not at all.

// crashBatch is how many records one write puts in one transaction.
const crashBatch = 4

// TestCrashChild is the child of the tests below. It does nothing when
// run on its own.
func TestCrashChild(t *testing.T) {
	crashtest.Serve(t, func(path, mode string, start int,
		ack func(int), say func(string)) {
		smallReclaim(t)
		s, err := Open(path, Options{Now: time.Now})
		if err != nil {
			panic(err)
		}
		if _, err := s.Claim(); err != nil {
			panic(err)
		}
		for i := start; ; i++ {
			if err := crashWrite(s, mode, i, say); err != nil {
				panic(err)
			}
			ack(i)
		}
	})
}

// crashWrite is write number i of the mode.
func crashWrite(s *Store, mode string, i int, say func(string)) error {
	switch mode {
	case "incidents":
		items := map[string]Item[string]{}
		for j := 0; j < crashBatch; j++ {
			items[crashKey(i, j)] = Item[string]{Value: crashValue(i)}
		}
		return IncidentRecords[string](s).PutAll(items)
	case "changes":
		var entries []Entry[string]
		for j := 0; j < crashBatch; j++ {
			entries = append(entries, Entry[string]{Entity: "changes",
				At: crashTime(i, j), Value: crashValue(i)})
		}
		return ChangeLog[string](s).AppendAll(entries)
	case "state":
		return state(s).Put(crashKey(i, 0), crashValue(i))
	default: // "rewrite"
		if err := state(s).Put(crashKey(i, 0), crashValue(i)); err != nil {
			return err
		}
		if i%3 != 0 {
			return nil
		}
		return crashRewrite(s, i, say)
	}
}

// crashRewrite fills the file with junk, deletes it, and rewrites the
// file, which copies the live data next to it and renames the copy.
func crashRewrite(s *Store, i int, say func(string)) error {
	junk := map[string]Item[string]{}
	keys := make([]string, 0, 1200)
	for j := 0; j < 1200; j++ {
		key := fmt.Sprintf("junk-%05d-%04d", i, j)
		junk[key] = Item[string]{Value: strings.Repeat("x", 4096)}
		keys = append(keys, key)
	}
	if err := state(s).PutAll(junk); err != nil {
		return err
	}
	if err := state(s).DeleteMany(keys); err != nil {
		return err
	}
	say("rewriting")
	_, err := s.Reclaim()
	return err
}

func crashKey(i, j int) string { return fmt.Sprintf("rec-%05d-%d", i, j) }
func crashValue(i int) string  { return fmt.Sprintf("value-%d", i) }
func crashTime(i, j int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).
		Add(time.Duration(i*crashBatch+j) * time.Second)
}

// crashRead returns how many writes of the mode are in the file, and
// fails when one is partial or has a wrong value.
func crashRead(t *testing.T, s *Store, mode string, upTo int) []bool {
	t.Helper()
	have := make([]bool, upTo+1)
	count := make([]int, upTo+1)
	note := func(i, j int, value string) {
		require.LessOrEqual(t, i, upTo, "a write nobody started")
		assert.Equal(t, crashValue(i), value)
		count[i]++
	}
	switch mode {
	case "incidents":
		require.NoError(t, IncidentRecords[string](s).Range("",
			func(key string, value string) error {
				var i, j int
				_, err := fmt.Sscanf(key, "rec-%d-%d", &i, &j)
				note(i, j, value)
				return err
			}))
	case "changes":
		require.NoError(t, ChangeLog[string](s).Range("changes",
			time.Time{}, time.Time{},
			func(at time.Time, value string) error {
				n := int(at.Sub(crashTime(0, 0)) / time.Second)
				note(n/crashBatch, n%crashBatch, value)
				return nil
			}))
	default:
		require.NoError(t, state(s).Range("rec-",
			func(key string, value string) error {
				var i, j int
				_, err := fmt.Sscanf(key, "rec-%d-%d", &i, &j)
				note(i, j, value)
				return err
			}))
	}
	want := crashBatch
	if mode == "state" || mode == "rewrite" {
		want = 1
	}
	for i, n := range count {
		require.Contains(t, []int{0, want}, n,
			"write %d is half there after the crash", i)
		have[i] = n == want
	}
	return have
}

// crashRounds kills the child round after round, each time after a
// different point, and checks the file after every kill.
func crashRounds(t *testing.T, mode string, rounds int, o crashtest.Options) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	next := 0
	for round := 0; round < rounds; round++ {
		o.Test, o.Mode, o.Path, o.Start = "TestCrashChild", mode, path, next
		o.AfterAck = next + 5 + 3*round
		o.Delay = time.Duration(round) * 3 * time.Millisecond
		acked := crashtest.Crash(t, o)

		s, err := Open(path, Options{Now: time.Now})
		require.NoError(t, err, "round %d: the file does not open", round)
		_, err = s.Claim()
		require.NoError(t, err)
		have := crashRead(t, s, mode, acked+crashBatch*2+400)
		for i := 0; i < next; i++ {
			require.True(t, have[i], "round %d: write %d lost", round, i)
		}
		for i := next; i <= acked; i++ {
			require.True(t, have[i], "round %d: acked write %d lost",
				round, i)
		}
		for i := acked + 1; i < len(have); i++ {
			if have[i] {
				next = i + 1
			}
		}
		next = max(next, acked+1)
		require.NoError(t, s.Close())
	}
}

func TestCrashDuringIncidentRecordWrite(t *testing.T) {
	crashRounds(t, "incidents", 6, crashtest.Options{})
}

func TestCrashDuringChangeStoreFlush(t *testing.T) {
	crashRounds(t, "changes", 6, crashtest.Options{})
}

func TestCrashDuringStateWrite(t *testing.T) {
	crashRounds(t, "state", 6, crashtest.Options{})
}

// The child is killed in the middle of the copy that a rewrite makes,
// at several moments after it announced the rewrite.
func TestCrashDuringOnlineRewrite(t *testing.T) {
	crashRounds(t, "rewrite", 8, crashtest.Options{AfterSay: "rewriting"})
	// AfterAck is ignored when AfterSay is set.
}

// A copy a crashed rewrite left next to the file is never used: the
// next rewrite starts it again, whatever it held, and the file keeps
// every write made since.
func TestRewriteIgnoresALeftoverCopy(t *testing.T) {
	smallReclaim(t)
	for name, leftover := range map[string]func(path string){
		"truncated": func(path string) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path+".compact",
				data[:len(data)/3], 0o600))
		},
		"garbage": func(path string) {
			require.NoError(t, os.WriteFile(path+".compact",
				[]byte("not a database"), 0o600))
		},
		"older copy": func(path string) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path+".compact", data, 0o600))
		},
	} {
		t.Run(name, func(t *testing.T) {
			clock := newFakeClock()
			s := openClaimed(t, clock)
			require.NoError(t, state(s).Put("rec-00000-0", crashValue(0)))
			path := s.path
			leftover(path)
			require.NoError(t, state(s).Put("rec-00001-0", crashValue(1)))
			fillAndEmpty(t, s, 1500)

			rewrote, err := s.Reclaim()

			require.NoError(t, err)
			assert.True(t, rewrote)
			for i := 0; i < 2; i++ {
				got, found, err := state(s).Get(crashKey(i, 0))
				require.NoError(t, err)
				assert.True(t, found, "write %d lost", i)
				assert.Equal(t, crashValue(i), got)
			}
			_, err = os.Stat(path + ".compact")
			assert.True(t, os.IsNotExist(err), "the copy is gone")
		})
	}
}

// The file copied while a write transaction is open, as a power cut
// would leave it, opens and holds only committed data.
func TestFileCopiedMidTransactionHoldsOnlyCommittedData(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	require.NoError(t, state(s).Put("rec-00000-0", crashValue(0)))

	copied := filepath.Join(t.TempDir(), "copy.db")
	err := s.update(func(tx *bolt.Tx) error {
		if err := tx.Bucket([]byte(State)).Put([]byte("rec-00001-0"),
			[]byte("half")); err != nil {
			return err
		}
		data, err := os.ReadFile(s.path)
		if err != nil {
			return err
		}
		return os.WriteFile(copied, data, 0o600)
	})
	require.NoError(t, err)

	again, err := Open(copied, Options{Now: clock.Now})
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })
	_, err = again.Claim()
	require.NoError(t, err)
	got, found, err := state(again).Get(crashKey(0, 0))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, crashValue(0), got)
	_, found, err = state(again).Get(crashKey(1, 0))
	require.NoError(t, err)
	assert.False(t, found, "the open transaction was never committed")
}
