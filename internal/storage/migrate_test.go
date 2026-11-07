package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

var migratedKey = []byte("migrated-by-test")

// withMigrations replaces the migration steps for one test.
func withMigrations(t *testing.T, steps ...migration) {
	t.Helper()
	saved := migrations
	migrations = steps
	t.Cleanup(func() { migrations = saved })
}

// markStep is an idempotent step that counts its runs in *runs and
// leaves a mark in the file's meta bucket.
func markStep(from uint64, runs *int) migration {
	return migration{From: from, Step: func(tx *bolt.Tx) error {
		*runs++
		return tx.Bucket(metaBucket).Put(migratedKey, []byte("yes"))
	}}
}

// fileFacts reads the version and the step's mark of the file at path.
func fileFacts(t *testing.T, path string) (uint64, bool) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true})
	require.NoError(t, err)
	defer db.Close()
	var version uint64
	var marked bool
	require.NoError(t, db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		version = readUint(meta.Get(versionKey))
		marked = meta.Get(migratedKey) != nil
		return nil
	}))
	return version, marked
}

func TestOpenMigratesAnOlderFileInPlace(t *testing.T) {
	for name, deferred := range map[string]bool{
		"open": false, "open then claim": true,
	} {
		t.Run(name, func(t *testing.T) {
			runs := 0
			withMigrations(t, markStep(SchemaVersion-1, &runs))
			path := filepath.Join(t.TempDir(), "state.db")
			seeded(t, path)
			writeVersion(t, path, SchemaVersion-1)

			s, err := Open(path, Options{Now: newFakeClock().Now,
				DeferRepair: deferred})
			require.NoError(t, err)
			claim(t, s)
			_, reset := s.Reset()
			assert.False(t, reset, "migrated, not reset")
			got, found, err := state(s).Get("k")
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, "old", got, "the data is kept")
			require.NoError(t, s.Close())

			version, marked := fileFacts(t, path)
			assert.Equal(t, uint64(SchemaVersion), version)
			assert.True(t, marked)
			assert.Equal(t, 1, runs)
			backup := fmt.Sprintf("%s%s%d", path, backupSuffix,
				SchemaVersion-1)
			_, err = os.Stat(backup)
			assert.NoError(t, err, "the old file is backed up")
			assert.Empty(t, keptCopiesOf(path), "nothing set aside")

			// A migrated file is current: the next open runs no step.
			again := openStoreAt(t, path)
			require.NoError(t, again.Close())
			assert.Equal(t, 1, runs)
		})
	}
}

func TestFailedMigrationResetsAndKeepsTheOldFile(t *testing.T) {
	ran := false
	withMigrations(t, migration{From: SchemaVersion - 1,
		Step: func(tx *bolt.Tx) error {
			ran = true
			// A step that wrote before failing is rolled back.
			if err := tx.Bucket(metaBucket).Put(migratedKey,
				[]byte("half")); err != nil {
				return err
			}
			return errors.New("bad record")
		}})
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)
	writeVersion(t, path, SchemaVersion-1)

	s := openStoreAt(t, path)
	defer s.Close()

	require.True(t, ran)
	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetMigrationFailed, reset.Reason)
	assert.Equal(t, uint64(SchemaVersion-1), reset.OldVersion)
	kept := keptCopiesOf(path)
	require.Len(t, kept, 1)
	version, marked := fileFacts(t, kept[0])
	assert.Equal(t, uint64(SchemaVersion-1), version,
		"the kept file is the one before the migration")
	assert.False(t, marked, "the failed step's write rolled back")
}

func TestIncompleteMigrationChainResets(t *testing.T) {
	runs := 0
	withMigrations(t, markStep(SchemaVersion-1, &runs))
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)
	writeVersion(t, path, SchemaVersion-2)

	s := openStoreAt(t, path)
	defer s.Close()

	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetSchemaMismatch, reset.Reason)
	assert.Zero(t, runs, "no step runs without a complete chain")
}

func TestMigrationChainOrder(t *testing.T) {
	var order []uint64
	step := func(from uint64) migration {
		return migration{From: from, Step: func(*bolt.Tx) error {
			order = append(order, from)
			return nil
		}}
	}
	withMigrations(t, step(SchemaVersion-1), step(SchemaVersion-2))
	chain, ok := migrationChain(SchemaVersion - 2)
	require.True(t, ok)
	require.Len(t, chain, 2)
	for _, m := range chain {
		require.NoError(t, m.Step(nil))
	}
	assert.Equal(t, []uint64{SchemaVersion - 2, SchemaVersion - 1}, order)
	_, ok = migrationChain(SchemaVersion + 1)
	assert.False(t, ok, "a newer file never migrates down")
}

// Earlier complete copies go once the new one is in place, and a copy a
// crash left half written goes before the next copy is made.
func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	old := path + backupSuffix + "1"
	cur := path + backupSuffix + "2"
	tmp := path + backupSuffix + "3" + backupTemp
	for _, name := range []string{old, cur, tmp} {
		if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups(path, "", true)
	exists := func(name string) bool {
		_, err := os.Stat(name)
		return err == nil
	}
	if exists(tmp) || !exists(old) || !exists(cur) {
		t.Fatalf("partial prune: tmp %v old %v cur %v",
			exists(tmp), exists(old), exists(cur))
	}
	pruneBackups(path, cur, false)
	if exists(old) || !exists(cur) {
		t.Fatalf("complete prune: old %v cur %v", exists(old), exists(cur))
	}
}

// A failed version read is an error the backup logs and goes on from;
// it is never swallowed into "nothing to migrate".
func TestReadVersionReportsAFailedRead(t *testing.T) {
	db, err := bolt.Open(filepath.Join(t.TempDir(), "v.db"), 0o600, nil)
	require.NoError(t, err)
	var v uint64
	var known bool
	require.NoError(t, readVersion(db, &v, &known))
	require.NoError(t, db.Close())
	require.Error(t, readVersion(db, &v, &known))
	// backupBeforeMigration on the unreadable file must not panic and
	// must not skip silently (it logs and tries the copy).
	backupBeforeMigration(db, filepath.Join(t.TempDir(), "x.db"), nil)
}
