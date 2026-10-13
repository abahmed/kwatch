package flight

import (
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/abahmed/kwatch/internal/storage"
)

// A state file from before the flight recorder has no flight bucket and
// no salt. It opens, the recorder makes a salt and starts empty, and the
// salt then stays across restarts.
func TestStateFileWithoutFlightBucketOrSaltRecordsFromEmpty(t *testing.T) {
	clock := newClock()
	path := t.TempDir() + "/state.db"
	old, err := storage.Open(path, storage.Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(storage.Flight))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store := openStore(t, path, clock.Now)
	salt, err := LoadSalt(store)
	if err != nil || salt == "" {
		t.Fatalf("salt = %q, %v", salt, err)
	}
	if got := readAll(t, NewRing(store)); len(got) != 0 {
		t.Fatalf("a new recording starts empty: %+v", got)
	}
	rec, err := NewRecorder(NewRing(store), salt, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Minute)
	submitAll(rec, podObserved("api-1", 1))
	rec.Flush()
	if got := readAll(t, NewRing(store)); len(got) != 1 {
		t.Fatalf("one observation recorded, got %+v", got)
	}
	if again, _ := LoadSalt(store); again != salt {
		t.Fatalf("the salt changed: %q then %q", salt, again)
	}
}
