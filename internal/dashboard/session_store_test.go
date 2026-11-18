package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// memSessionStore is a state file for sessions: it keeps the JSON a
// real store would.
type memSessionStore struct {
	raw     []byte
	saves   int
	loadErr error
}

func (m *memSessionStore) LoadSessions() (StoredSessions, bool, error) {
	var out StoredSessions
	if m.loadErr != nil {
		return out, false, m.loadErr
	}
	if m.raw == nil {
		return out, false, nil
	}
	err := json.Unmarshal(m.raw, &out)
	return out, err == nil, err
}

func (m *memSessionStore) SaveSessions(state StoredSessions) error {
	raw, err := json.Marshal(state)
	m.raw, m.saves = raw, m.saves+1
	return err
}

func (m *memSessionStore) state(t *testing.T) StoredSessions {
	t.Helper()
	state, found, err := m.LoadSessions()
	if err != nil || !found {
		t.Fatalf("nothing stored: %v", err)
	}
	return state
}

// cheapChecks makes the credential digest cheap for the test.
func cheapChecks(t *testing.T) {
	t.Helper()
	old := credentialRounds
	credentialRounds = 64
	t.Cleanup(func() { credentialRounds = old })
}

// storedServer is a kwatch start over store at the test clock.
func storedServer(
	store SessionStore, password string, at time.Time,
) *sessionServer {
	s := newSessionServer(false, password)
	s.now = at
	s.SetSessionStore(store)
	return s
}

func logout(s *sessionServer, c *http.Cookie) {
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(c)
	s.do(req)
}

// A sign-in outlives a restart over the same state file, up to its
// 12 hours.
func TestSessionSurvivesARestartWithAStore(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	first := storedServer(store, testPassword, testEpoch)
	c := first.signIn(t, testPassword)

	again := storedServer(store, testPassword, testEpoch.Add(time.Hour))
	if code := again.getWith("/api/incidents", c); code != 200 {
		t.Fatalf("after a restart: %d, want 200", code)
	}
	again.now = testEpoch.Add(12*time.Hour - time.Second)
	if code := again.getWith("/api/session", c); code != 204 {
		t.Errorf("before 12h: %d, want 204", code)
	}
	again.now = testEpoch.Add(12 * time.Hour)
	if code := again.getWith("/api/incidents", c); code != 401 {
		t.Errorf("at 12h: %d, want 401", code)
	}
	later := storedServer(store, testPassword, testEpoch.Add(13*time.Hour))
	if n := len(store.state(t).Sessions); n != 0 {
		t.Errorf("expired sessions kept: %d", n)
	}
	if code := later.getWith("/api/incidents", c); code != 401 {
		t.Errorf("expired after a restart: %d, want 401", code)
	}
}

// Signing out ends the session at once, and it stays ended after a
// restart.
func TestSessionLogoutWithAStoreSurvivesARestart(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	out, kept := s.signIn(t, testPassword), s.signIn(t, testPassword)
	logout(s, out)
	if code := s.getWith("/api/incidents", out); code != 401 {
		t.Errorf("signed out: %d, want 401", code)
	}
	again := storedServer(store, testPassword, testEpoch.Add(time.Minute))
	if code := again.getWith("/api/incidents", out); code != 401 {
		t.Errorf("signed out, then restart: %d, want 401", code)
	}
	if code := again.getWith("/api/incidents", kept); code != 200 {
		t.Errorf("other session after restart: %d, want 200", code)
	}
	logout(again, kept)
	third := storedServer(store, testPassword, testEpoch.Add(2*time.Minute))
	if code := third.getWith("/api/incidents", kept); code != 401 {
		t.Errorf("signed out after a restart: %d, want 401", code)
	}
}

// A new password or token signs everyone out, also across restarts,
// and changing it back does not bring old sessions back.
func TestSessionCredentialChangeWithAStoreSignsEveryoneOut(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	c := s.signIn(t, testPassword)

	changed := storedServer(store, "NEWP-ASSW-ORD0-0000", testEpoch)
	if code := changed.getWith("/api/incidents", c); code != 401 {
		t.Errorf("new password: %d, want 401", code)
	}
	back := storedServer(store, testPassword, testEpoch)
	if code := back.getWith("/api/incidents", c); code != 401 {
		t.Errorf("password changed back: %d, want 401", code)
	}

	live := back.signIn(t, testPassword)
	back.SetCredentials("other-token", testPassword)
	if code := back.getWith("/api/incidents", live); code != 401 {
		t.Errorf("new token while running: %d, want 401", code)
	}
	if n := len(store.state(t).Sessions); n != 0 {
		t.Errorf("sessions kept after a credential change: %d", n)
	}
	fresh := back.signIn(t, testPassword)
	restarted := newSessionServer(false, testPassword)
	restarted.Server.auth.setCredentials("other-token", testPassword)
	restarted.SetSessionStore(store)
	if code := restarted.getWith("/api/incidents", fresh); code != 200 {
		t.Errorf("new credentials, restart: %d, want 200", code)
	}
}

// A changed cookie is refused, whatever the store holds.
func TestSessionTamperedCookieWithAStoreIsRefused(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	c := s.signIn(t, testPassword)
	payload, sig, _ := strings.Cut(c.Value, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	raw[len(raw)-1] ^= 1
	forged := *c
	forged.Value = base64.RawURLEncoding.EncodeToString(raw) + "." + sig
	again := storedServer(store, testPassword, testEpoch)
	for _, srv := range []*sessionServer{s, again} {
		if code := srv.getWith("/api/incidents", &forged); code != 401 {
			t.Errorf("forged cookie: %d, want 401", code)
		}
	}
}

// The state file keeps neither the password, the token nor a session
// ID: only the seed, the slow check and hashes of the IDs.
func TestSessionStoreKeepsNoSecretsAtRest(t *testing.T) {
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	c := s.signIn(t, testPassword)
	payload, sig, _ := strings.Cut(c.Value, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	nonce := raw[sessionNonceAt:]
	text := string(store.raw)
	for _, secret := range []string{testPassword, testToken, payload, sig,
		base64.StdEncoding.EncodeToString(nonce),
		strings.ToLower(strings.ReplaceAll(testPassword, "-", ""))} {
		if strings.Contains(text, secret) {
			t.Errorf("state holds a secret %q", secret)
		}
	}
	state := store.state(t)
	if _, ok := state.Sessions[recordID(nonce)]; !ok ||
		len(state.Sessions) != 1 || len(state.Key) != storedKeyBytes ||
		len(state.Check) != 16 {
		t.Errorf("stored state = %+v", state)
	}
}

// At most maxStoredSessions are kept: a new sign-in ends the oldest.
func TestSessionStoreIsBounded(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	first := s.signIn(t, testPassword)
	for i := 1; i <= maxStoredSessions; i++ {
		s.now = testEpoch.Add(time.Duration(i) * time.Second)
		if _, _, err := s.auth.sessions.issue(); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(store.state(t).Sessions); n != maxStoredSessions {
		t.Errorf("stored %d sessions, want %d", n, maxStoredSessions)
	}
	if code := s.getWith("/api/incidents", first); code != 401 {
		t.Errorf("oldest session: %d, want 401", code)
	}
}

// A new or reset state file signs everyone out, and the sign-in page
// says kwatch restarted.
func TestSessionResetStateFileSignsEveryoneOut(t *testing.T) {
	cheapChecks(t)
	s := storedServer(&memSessionStore{}, testPassword, testEpoch)
	c := s.signIn(t, testPassword)
	reset := storedServer(&memSessionStore{}, testPassword, testEpoch)
	if code := reset.getWith("/api/incidents", c); code != 401 {
		t.Errorf("reset state file: %d, want 401", code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(c)
	if body := reset.do(req).Body.String(); !strings.Contains(body,
		`"reason":"restarted"`) {
		t.Errorf("sign-in page answer = %s", body)
	}
}

// While no state file is attached, sessions live in memory and are
// written at the next attach; a store that cannot be read keeps
// today's memory mode.
func TestSessionStoreDetachAndUnreadableStore(t *testing.T) {
	cheapChecks(t)
	store := &memSessionStore{}
	s := storedServer(store, testPassword, testEpoch)
	s.SetSessionStore(nil)
	c := s.signIn(t, testPassword)
	saves := store.saves
	if code := s.getWith("/api/incidents", c); code != 200 {
		t.Errorf("detached: %d, want 200", code)
	}
	s.SetSessionStore(store)
	if store.saves != saves+1 {
		t.Errorf("reattach saved %d times", store.saves-saves)
	}
	again := storedServer(store, testPassword, testEpoch)
	if code := again.getWith("/api/incidents", c); code != 200 {
		t.Errorf("signed in while detached, restart: %d", code)
	}

	broken := &memSessionStore{loadErr: errors.New("disk gone")}
	m := storedServer(broken, testPassword, testEpoch)
	mc := m.signIn(t, testPassword)
	if code := m.getWith("/api/incidents", mc); code != 200 ||
		broken.saves != 0 {
		t.Errorf("unreadable store: %d, %d saves", code, broken.saves)
	}
}
