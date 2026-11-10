package dashboard

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// testPrivate is the browser's private key of RFC 8291's example,
// whose public half is testP256dh.
const testPrivate = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"

// pushService is a fake push service: it keeps the bodies it got.
type pushService struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (f *pushService) take() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.bodies
	f.bodies = nil
	return out
}

// newPushService starts a fake push service; every address the
// returned sender dials reaches it.
func newPushService(t *testing.T) (*pushService, transport.Sender) {
	t.Helper()
	f := &pushService{}
	ts := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.bodies = append(f.bodies, b)
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		}))
	t.Cleanup(ts.Close)
	addr := ts.Listener.Addr().String()
	dial := func(ctx context.Context, network, _ string) (net.Conn,
		error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	return f, transport.NewSender(transport.Dependencies{
		HTTPClient: client, Clock: clock.Func(func() time.Time {
			return testEpoch
		})})
}

// openPush decrypts an aes128gcm body (RFC 8291) sent to testSub.
func openPush(t *testing.T, body []byte) pushMessage {
	t.Helper()
	dec := base64.RawURLEncoding.DecodeString
	priv, _ := dec(testPrivate)
	auth, _ := dec(testAuth)
	ua, err := ecdh.P256().NewPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	salt, idlen := body[:16], int(body[20])
	key := body[21 : 21+idlen]
	sender, err := ecdh.P256().NewPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	shared, _ := ua.ECDH(sender)
	info := append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...)
	info = append(info, key...)
	ikm, _ := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		t.Fatal(err)
	}
	var m pushMessage
	if err := json.Unmarshal(plain[:len(plain)-1], &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// An issue is news when it is confirmed, not while it settles; one
// that settles quietly is never pushed, and the icon counts neither.
func TestPushTellsOnlyConfirmedIssues(t *testing.T) {
	svc, send := newPushService(t)
	book, err := loadPushBook(openPushStore(t), "gen1", testEpoch)
	if err != nil {
		t.Fatal(err)
	}
	p := &pusher{book: book, send: send}
	_ = p.book.subscribe(testSub(chrome),
		pushPrefs{Min: "digest", Resolved: true}, "", testEpoch)
	untold := func(id, sev, state string) IncidentListItem {
		it := pushTestItem(id, sev, state)
		it.Untold = true
		return it
	}
	ctx := context.Background()
	old := pushTestItem("a", "page", "open")
	p.check(ctx, items(old), testEpoch)
	p.check(ctx, items(old, untold("s", "notify", "settling"),
		untold("q", "page", "settling")), testEpoch)
	if got := svc.take(); len(got) != 0 {
		t.Fatalf("settling issues sent %d", len(got))
	}
	p.check(ctx, items(old, pushTestItem("s", "notify", "open"),
		untold("q", "page", "settling")), testEpoch)
	got := svc.take()
	if len(got) != 1 {
		t.Fatalf("a confirmed issue sent %d, want 1", len(got))
	}
	if m := openPush(t, got[0]); m.Badge == nil || *m.Badge != 1 {
		t.Errorf("badge = %v, want 1 (settling left out)", m.Badge)
	}
	p.check(ctx, items(old, pushTestItem("s", "notify", "open"),
		untold("q", "page", stateResolved)), testEpoch)
	if got := svc.take(); len(got) != 0 {
		t.Errorf("a quiet settle sent %d", len(got))
	}
}
