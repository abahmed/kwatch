package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// putRaw writes data under key without the store's encoding.
func putRaw(t *testing.T, s *Store, b Bucket, key, data []byte) {
	t.Helper()
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(b)).Put(key, data)
	}))
}

func TestKeyedCorruptValueIsSkippedAndCounted(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	values := IncidentRecords[record](s)
	require.NoError(t, values.Put("a", record{Name: "a"}))
	require.NoError(t, values.Put("c", record{Name: "c"}))
	putRaw(t, s, Incidents, []byte("b"), []byte("short"))
	good, err := encode(record{}, time.Now(), time.Time{})
	require.NoError(t, err)
	putRaw(t, s, Incidents, []byte("b2"), append(good[:headerSize],
		[]byte("{not json")...))

	var names []string
	require.NoError(t, values.Range("", func(_ string, r record) error {
		names = append(names, r.Name)
		return nil
	}))
	_, found, err := values.Get("b")

	require.NoError(t, err)
	assert.False(t, found, "a corrupt value reads as absent")
	assert.Equal(t, []string{"a", "c"}, names)
	assert.Equal(t, uint64(3), s.Stats().CorruptRecords)
}

func TestLogCorruptEntryIsSkippedAndCounted(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	l := ChangeLog[string](s)
	require.NoError(t, l.Append("pod", minute(0), "ok"))
	putRaw(t, s, Changes, logKey("pod", minute(1), 99), []byte("x"))
	require.NoError(t, l.Append("pod", minute(2), "later"))

	assert.Equal(t, []string{"ok", "later"},
		collect(t, l, "pod", time.Time{}, time.Time{}))
	assert.Equal(t, uint64(1), s.Stats().CorruptRecords)
}

func TestCorruptValueIsLoggedOncePerBucket(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	putRaw(t, s, State, []byte("bad"), []byte("x"))
	putRaw(t, s, Threads, []byte("bad"), []byte("x"))
	for i := 0; i < 3; i++ {
		_, _, err := state(s).Get("bad")
		require.NoError(t, err)
	}
	_, _, err := ThreadValues[string](s).Get("bad")
	require.NoError(t, err)

	logged := 0
	s.counters.logged.Range(func(any, any) bool {
		logged++
		return true
	})
	assert.Equal(t, 2, logged, "one log line per bucket")
	assert.Equal(t, uint64(4), s.Stats().CorruptRecords)
}
