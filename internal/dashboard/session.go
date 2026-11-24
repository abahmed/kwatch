package dashboard

import (
	"container/heap"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// sessionLifetime is how long a sign-in lasts, from sign-in, without
// sliding (decision D10), unless Options.SessionLifetime sets another.
const sessionLifetime = 12 * time.Hour

// maxRevoked bounds the signed-out sessions remembered until they
// expire. At the bound the next sign-out rotates the signing key (see
// revoke) rather than forgetting a revocation.
const maxRevoked = 4096

// sessionCookie names the session cookie.
const sessionCookie = "kwatch_session"

// sessions issues and checks signed session cookies. A cookie is the
// sign-in time, a tag of the signing key's seed and a random nonce,
// signed with a key derived from the password, the token and the
// seed: a new password signs everyone out. In memory mode the seed is
// drawn at each start, so a restart signs everyone out too, and
// nothing is stored per session except the nonces of sessions signed
// out early, kept until they would have expired. A revocation is never
// forgotten early: when the table is full the signing key rotates
// instead, which signs every session out and empties the table. Once a
// state store is attached (session_store.go) the seed and a record per
// session are kept in it, and sessions survive a restart.
type sessions struct {
	key      atomic.Pointer[[]byte]
	now      clock.Clock
	disabled atomic.Bool
	// lifetime is how long a sign-in lasts; set before serving.
	lifetime time.Duration

	mu   sync.Mutex
	boot [bootTagBytes]byte
	// stored keeps the sessions in the state store; nil in memory
	// mode (session_store.go).
	stored  *storedSessions
	revoked map[string]time.Time
	// expiry orders the revoked nonces by when they expire, so pruning
	// pops only what has expired instead of scanning the table.
	expiry expiryHeap
	// saveMu orders the writes to the state store; attachMu the
	// attaches.
	saveMu   sync.Mutex
	attachMu sync.Mutex
}

func newSessions(password, token string, now clock.Clock) *sessions {
	bootKey := make([]byte, 32)
	if _, err := rand.Read(bootKey); err != nil {
		// crypto/rand does not fail on supported platforms; refuse
		// every cookie rather than sign with a guessable key.
		bootKey = nil
	}
	key := deriveKey(bootKey, password, token)
	s := &sessions{now: now, revoked: make(map[string]time.Time),
		lifetime: sessionLifetime}
	s.disabled.Store(bootKey == nil)
	s.key.Store(&key)
	s.boot = bootTag(bootKey)
	return s
}

// deriveKey is the signing key of seed and the sign-in secrets.
func deriveKey(seed []byte, password, token string) []byte {
	mac := hmac.New(sha256.New, []byte("kwatch-dashboard-session"))
	mac.Write(seed)
	mac.Write([]byte(token))
	mac.Write([]byte{0})
	mac.Write([]byte(password))
	return mac.Sum(nil)
}

// bootTag names the seed in each cookie, so a cookie of another seed
// (an earlier start, or a reset state file) can be told apart.
func bootTag(seed []byte) [bootTagBytes]byte {
	var out [bootTagBytes]byte
	tag := sha256.Sum256(append([]byte("boot"), seed...))
	copy(out[:], tag[:])
	return out
}

// expiryItem is a revoked nonce and when it expires.
type expiryItem struct {
	nonce string
	until time.Time
}

type expiryHeap []expiryItem

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].until.Before(h[j].until) }
func (h expiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *expiryHeap) Push(x any)        { *h = append(*h, x.(expiryItem)) }
func (h *expiryHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	*h = old[:len(old)-1]
	return item
}

const (
	bootTagBytes        = 4
	sessionNonceAt      = 8 + bootTagBytes
	sessionPayloadBytes = sessionNonceAt + 16
	sessionSigBytes     = sha256.Size
)

// issue returns a new cookie value and when it expires.
func (s *sessions) issue() (string, time.Time, error) {
	payload := make([]byte, sessionPayloadBytes)
	at := s.now.Now()
	binary.BigEndian.PutUint64(payload, uint64(at.Unix()))
	if _, err := rand.Read(payload[sessionNonceAt:]); err != nil {
		return "", time.Time{}, err
	}
	s.mu.Lock()
	copy(payload[8:], s.boot[:])
	enc := base64.RawURLEncoding
	value := enc.EncodeToString(payload) + "." +
		enc.EncodeToString(s.sign(payload))
	stored := s.stored != nil
	if stored {
		s.stored.add(payload[sessionNonceAt:], at.Add(s.lifetime), at)
	}
	s.mu.Unlock()
	if stored {
		s.save()
	}
	return value, at.Add(s.lifetime), nil
}

func (s *sessions) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, *s.key.Load())
	mac.Write(payload)
	return mac.Sum(nil)
}

// parse returns the payload of a correctly signed cookie value.
func (s *sessions) parse(value string) ([]byte, bool) {
	rawPayload, rawSig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, false
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(rawPayload)
	if err != nil || len(payload) != sessionPayloadBytes {
		return nil, false
	}
	sig, err := enc.DecodeString(rawSig)
	if err != nil || len(sig) != sessionSigBytes {
		return nil, false
	}
	if !hmac.Equal(sig, s.sign(payload)) {
		return nil, false
	}
	return payload, true
}

// valid reports whether value is a live session: signed with the current
// key, not older than its lifetime, not from the future and not
// signed out.
func (s *sessions) valid(value string) bool {
	payload, ok := s.parse(value)
	if !ok || s.disabled.Load() {
		return false
	}
	issued := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	now := s.now.Now()
	if now.Before(issued.Add(-time.Minute)) ||
		!now.Before(issued.Add(s.lifetime)) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stored != nil {
		return s.stored.has(payload[sessionNonceAt:], now)
	}
	_, out := s.revoked[string(payload[sessionNonceAt:])]
	return !out
}

// revoke signs a session out before it expires. Expired entries are
// popped first; if the table is still full the signing key rotates,
// which signs every session out (the revoked one included), so no
// revocation is ever forgotten while its session could still be used.
func (s *sessions) revoke(value string) {
	payload, ok := s.parse(value)
	if !ok {
		return
	}
	if s.forgetStored(payload[sessionNonceAt:]) {
		return
	}
	issued := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	until := issued.Add(s.lifetime)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpired()
	if len(s.revoked) >= maxRevoked {
		s.rotateKey()
		return
	}
	nonce := string(payload[sessionNonceAt:])
	s.revoked[nonce] = until
	heap.Push(&s.expiry, expiryItem{nonce: nonce, until: until})
}

// pruneExpired forgets revocations whose sessions have expired anyway.
// The caller holds s.mu.
func (s *sessions) pruneExpired() {
	now := s.now.Now()
	for len(s.expiry) > 0 && !now.Before(s.expiry[0].until) {
		item := heap.Pop(&s.expiry).(expiryItem)
		delete(s.revoked, item.nonce)
	}
}

// rotateKey replaces the signing key with one derived from the old key
// and fresh randomness and empties the revocation table. The caller
// holds s.mu.
func (s *sessions) rotateKey() {
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		s.disabled.Store(true)
		return
	}
	mac := hmac.New(sha256.New, *s.key.Load())
	mac.Write(fresh)
	key := mac.Sum(nil)
	s.key.Store(&key)
	s.revoked = make(map[string]time.Time)
	s.expiry = nil
}

// rekey signs with a key for new sign-in secrets: every session of the
// old ones is signed out and the revocation table emptied.
func (s *sessions) rekey(password, token string) {
	if s.rekeyStored(password, token) {
		return
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		s.disabled.Store(true)
		return
	}
	key := deriveKey(fresh, password, token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key.Store(&key)
	s.revoked = make(map[string]time.Time)
	s.expiry = nil
}

// fromEarlierStart reports whether value is a well-formed cookie of
// this server's kind that was issued before this start: its signature
// cannot be checked any more, but its boot tag differs. The sign-in
// page uses it to say the person was signed out by a restart.
func (s *sessions) fromEarlierStart(value string) bool {
	rawPayload, rawSig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(rawPayload)
	if err != nil || len(payload) != sessionPayloadBytes {
		return false
	}
	if sig, err := enc.DecodeString(rawSig); err != nil ||
		len(sig) != sessionSigBytes {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(payload[8:sessionNonceAt]) != string(s.boot[:])
}
