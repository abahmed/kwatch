package dashboard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// sessionLifetime is how long a sign-in lasts, from sign-in, without
// sliding (decision D10).
const sessionLifetime = 12 * time.Hour

// maxRevoked bounds the signed-out sessions remembered until they expire.
const maxRevoked = 4096

// sessionCookie names the session cookie.
const sessionCookie = "kwatch_session"

// sessions issues and checks signed session cookies. A cookie is the
// sign-in time and a random nonce, signed with a key derived from the
// password, the token and a random value drawn at each start: a new
// password and a kwatch restart both sign everyone out. Nothing is
// stored per session except the nonces of sessions signed out early.
// That revocation is best-effort and in memory: it is bounded by
// maxRevoked, evicting the oldest, and a restart clears it, which is
// safe only because a restart also changes the key.
type sessions struct {
	key      []byte
	now      clock.Clock
	disabled bool

	mu      sync.Mutex
	revoked map[string]time.Time
}

func newSessions(password, token string, now clock.Clock) *sessions {
	boot := make([]byte, 32)
	if _, err := rand.Read(boot); err != nil {
		// crypto/rand does not fail on supported platforms; refuse
		// every cookie rather than sign with a guessable key.
		boot = nil
	}
	mac := hmac.New(sha256.New, []byte("kwatch-dashboard-session"))
	mac.Write(boot)
	mac.Write([]byte(token))
	mac.Write([]byte{0})
	mac.Write([]byte(password))
	return &sessions{key: mac.Sum(nil), now: now, disabled: boot == nil,
		revoked: make(map[string]time.Time)}
}

const (
	sessionPayloadBytes = 8 + 16
	sessionSigBytes     = sha256.Size
)

// issue returns a new cookie value and when it expires.
func (s *sessions) issue() (string, time.Time, error) {
	payload := make([]byte, sessionPayloadBytes)
	at := s.now.Now()
	binary.BigEndian.PutUint64(payload, uint64(at.Unix()))
	if _, err := rand.Read(payload[8:]); err != nil {
		return "", time.Time{}, err
	}
	enc := base64.RawURLEncoding
	value := enc.EncodeToString(payload) + "." +
		enc.EncodeToString(s.sign(payload))
	return value, at.Add(sessionLifetime), nil
}

func (s *sessions) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.key)
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
// key, not older than sessionLifetime, not from the future and not
// signed out.
func (s *sessions) valid(value string) bool {
	payload, ok := s.parse(value)
	if !ok || s.disabled {
		return false
	}
	issued := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	now := s.now.Now()
	if now.Before(issued.Add(-time.Minute)) ||
		!now.Before(issued.Add(sessionLifetime)) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, out := s.revoked[string(payload[8:])]
	return !out
}

// revoke signs a session out before it expires.
func (s *sessions) revoke(value string) {
	payload, ok := s.parse(value)
	if !ok {
		return
	}
	issued := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	now := s.now.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for nonce, until := range s.revoked {
		if !now.Before(until) {
			delete(s.revoked, nonce)
		}
	}
	if len(s.revoked) >= maxRevoked {
		s.evictOldest()
	}
	s.revoked[string(payload[8:])] = issued.Add(sessionLifetime)
}

// evictOldest forgets the revoked session that expires first. The
// caller holds s.mu.
func (s *sessions) evictOldest() {
	oldest, first := "", time.Time{}
	for nonce, until := range s.revoked {
		if oldest == "" || until.Before(first) {
			oldest, first = nonce, until
		}
	}
	delete(s.revoked, oldest)
}
