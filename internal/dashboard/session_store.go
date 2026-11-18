package dashboard

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"time"

	"k8s.io/klog/v2"
)

// SessionStore keeps the dashboard's sessions in kwatch's state file,
// so a restart does not sign everyone out. The application supplies it
// while it holds the state file (Server.SetSessionStore).
type SessionStore interface {
	// LoadSessions reads what SaveSessions wrote; found is false when
	// nothing was saved (a new or reset state file).
	LoadSessions() (state StoredSessions, found bool, err error)
	SaveSessions(state StoredSessions) error
}

// StoredSessions is what is kept at rest. Key is the random seed of
// the signing key, which is also derived from the password and the
// token, so it signs nothing alone. Check is a slow digest of the
// password and token (PBKDF2), so sessions of other credentials are
// dropped on load. Sessions maps the SHA-256 of each session ID to when
// it expires. Neither a password nor a session ID is ever kept. Its
// JSON shape is persisted.
type StoredSessions struct {
	Key      []byte               `json:"key"`
	Check    []byte               `json:"check"`
	Sessions map[string]time.Time `json:"sessions"`
}

const (
	// maxStoredSessions bounds the sessions kept; at the bound a new
	// sign-in ends the session closest to expiry.
	maxStoredSessions = 1024
	// storedKeyBytes is the size of the stored seed.
	storedKeyBytes = 32
)

// credentialRounds is the PBKDF2 work of the credential check. It is
// a variable so tests can make it cheap.
var credentialRounds = 100_000

// storedSessions is the store-backed record of live sessions. It is
// guarded by sessions.mu.
type storedSessions struct {
	// store is nil while no state file is attached; the records live
	// in memory and are written at the next attach.
	store   SessionStore
	seed    []byte
	check   []byte
	records map[string]time.Time
}

// recordID is how a session is named at rest: the SHA-256 of its
// nonce.
func recordID(nonce []byte) string {
	sum := sha256.Sum256(nonce)
	return hex.EncodeToString(sum[:])
}

func (r *storedSessions) has(nonce []byte, now time.Time) bool {
	until, ok := r.records[recordID(nonce)]
	return ok && now.Before(until)
}

// add records a new session, making room within maxStoredSessions:
// expired sessions go first, then those closest to expiry.
func (r *storedSessions) add(nonce []byte, until, now time.Time) {
	r.prune(now)
	r.trim(maxStoredSessions - 1)
	r.records[recordID(nonce)] = until
}

// trim ends the sessions closest to expiry until at most limit are
// left.
func (r *storedSessions) trim(limit int) {
	extra := len(r.records) - limit
	if extra <= 0 {
		return
	}
	ids := slices.Collect(maps.Keys(r.records))
	slices.SortFunc(ids, func(a, b string) int {
		return r.records[a].Compare(r.records[b])
	})
	for _, id := range ids[:extra] {
		delete(r.records, id)
	}
}

func (r *storedSessions) prune(now time.Time) {
	for id, until := range r.records {
		if !now.Before(until) {
			delete(r.records, id)
		}
	}
}

func (r *storedSessions) snapshot() StoredSessions {
	out := StoredSessions{Key: bytes.Clone(r.seed),
		Check:    bytes.Clone(r.check),
		Sessions: make(map[string]time.Time, len(r.records))}
	for id, until := range r.records {
		out.Sessions[id] = until
	}
	return out
}

// credentialCheck is the slow digest of the sign-in secrets under
// seed; nil if it cannot be made.
func credentialCheck(seed []byte, password, token string) []byte {
	salt := sha256.Sum256(append([]byte("kwatch-dashboard-check"), seed...))
	out, err := pbkdf2.Key(sha256.New, token+"\x00"+password, salt[:],
		credentialRounds, 16)
	if err != nil {
		return nil
	}
	return out
}

// save writes the records to the attached store, if any. A failed
// write is logged without session data: the session still works until
// kwatch restarts.
func (s *sessions) save() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	if s.stored == nil || s.stored.store == nil {
		s.mu.Unlock()
		return
	}
	store, state := s.stored.store, s.stored.snapshot()
	s.mu.Unlock()
	if err := store.SaveSessions(state); err != nil {
		klog.V(1).InfoS("dashboard sessions not saved", "error", err)
	}
}

// forgetStored signs a stored session out: its record is removed and
// saved. It reports false in memory mode.
func (s *sessions) forgetStored(nonce []byte) bool {
	s.mu.Lock()
	stored := s.stored != nil
	if stored {
		delete(s.stored.records, recordID(nonce))
	}
	s.mu.Unlock()
	if stored {
		s.save()
	}
	return stored
}

// rekeyStored signs with the stored seed and new sign-in secrets:
// every session is signed out. It reports false in memory mode.
func (s *sessions) rekeyStored(password, token string) bool {
	s.mu.Lock()
	if s.stored == nil {
		s.mu.Unlock()
		return false
	}
	seed := s.stored.seed
	s.mu.Unlock()
	check := credentialCheck(seed, password, token)
	key := deriveKey(seed, password, token)
	s.mu.Lock()
	s.key.Store(&key)
	s.stored.check = check
	s.stored.records = make(map[string]time.Time)
	s.mu.Unlock()
	s.save()
	return true
}

// attach keeps the sessions in store from now on. On the first attach
// of this process it loads the stored seed and sessions, or starts
// new ones on a new or reset state file, and signs with that seed:
// sessions signed in before it end. Later attaches (a new term as
// leader) write what this process holds, which is newer. nil detaches;
// the records stay in memory.
func (s *sessions) attach(store SessionStore, password, token string) {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	s.mu.Lock()
	if s.stored != nil || store == nil {
		if s.stored != nil {
			s.stored.store = store
		}
		s.mu.Unlock()
		s.save()
		return
	}
	s.mu.Unlock()
	stored, ok := loadStored(store, password, token, s.now.Now())
	if !ok {
		return
	}
	key := deriveKey(stored.seed, password, token)
	s.mu.Lock()
	s.key.Store(&key)
	s.boot = bootTag(stored.seed)
	s.stored = stored
	s.revoked = make(map[string]time.Time)
	s.expiry = nil
	s.mu.Unlock()
	s.save()
}

// loadStored reads the stored sessions, keeping only those of the
// current credentials that have not expired; a new seed when none is
// stored. false keeps memory mode: the store could not be read.
func loadStored(
	store SessionStore, password, token string, now time.Time,
) (*storedSessions, bool) {
	state, found, err := store.LoadSessions()
	if err != nil {
		klog.V(1).InfoS("dashboard sessions not loaded", "error", err)
		return nil, false
	}
	if !found || len(state.Key) != storedKeyBytes {
		state = StoredSessions{Key: make([]byte, storedKeyBytes)}
		if _, err := rand.Read(state.Key); err != nil {
			return nil, false
		}
	}
	check := credentialCheck(state.Key, password, token)
	r := &storedSessions{store: store, seed: state.Key, check: check,
		records: make(map[string]time.Time)}
	if check != nil && bytes.Equal(check, state.Check) {
		for id, until := range state.Sessions {
			r.records[id] = until
		}
	}
	r.prune(now)
	r.trim(maxStoredSessions)
	return r, true
}

// SetSessionStore keeps the dashboard's sessions in store while kwatch
// holds its state file, so they survive a restart; nil detaches it and
// the sessions stay in memory. Without a store (no state file)
// sessions end when kwatch restarts.
func (s *Server) SetSessionStore(store SessionStore) {
	secret := s.auth.secret()
	s.auth.sessions.attach(store, secret.password, secret.token)
}
