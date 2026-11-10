package dashboard

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/delivery/webpush"
	"github.com/abahmed/kwatch/internal/storage"
)

// A browser's keys (RFC 8291's example).
const (
	testP256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcx" +
		"aOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	testAuth = "BTBZMqHH6r4Tts7J_aSIgg"
)

func testSub(endpoint string) webpush.Subscription {
	return webpush.Subscription{Endpoint: endpoint,
		Keys: webpush.Keys{P256dh: testP256dh, Auth: testAuth}}
}

// fakePush records sends and answers with fail[endpoint].
type fakePush struct {
	sent []transport.Request
	fail map[string]error
}

func (f *fakePush) Send(
	_ context.Context, r transport.Request,
) ([]byte, error) {
	f.sent = append(f.sent, r)
	return nil, f.fail[r.URL]
}

func (f *fakePush) to(endpoint string) int {
	n := 0
	for _, r := range f.sent {
		if r.URL == endpoint {
			n++
		}
	}
	return n
}

// items lists the issues of one check; never nil.
func items(list ...IncidentListItem) func() []IncidentListItem {
	return func() []IncidentListItem {
		return append([]IncidentListItem{}, list...)
	}
}

func openPushStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(t.TempDir()+"/state.db",
		storage.Options{Now: func() time.Time { return testEpoch }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Claim(); err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestPusher(t *testing.T, store *storage.Store) (*pusher, *fakePush) {
	t.Helper()
	book, err := loadPushBook(store, "gen1", testEpoch)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePush{fail: map[string]error{}}
	return &pusher{book: book, send: f}, f
}

const (
	chrome  = "https://fcm.googleapis.com/fcm/send/one"
	chrome2 = "https://fcm.googleapis.com/fcm/send/two"
	firefox = "https://updates.push.services.mozilla.com/wpush/v2/x"
)

func TestPusherSendsEachBrowserWhatItAskedFor(t *testing.T) {
	p, f := newTestPusher(t, openPushStore(t))
	_ = p.book.subscribe(testSub(chrome), pushPrefs{Min: "page"}, "", testEpoch)
	_ = p.book.subscribe(testSub(firefox), pushPrefs{Min: "notify"}, "",
		testEpoch)
	ctx := context.Background()
	p.check(ctx, items(pushTestItem("a", "notify", "open")),
		testEpoch)
	if len(f.sent) != 0 {
		t.Fatalf("the first check sent %d", len(f.sent))
	}
	p.check(ctx, items(pushTestItem("a", "notify", "open"),
		pushTestItem("b", "notify", "open")), testEpoch)
	if f.to(firefox) != 1 || f.to(chrome) != 0 {
		t.Fatalf("new notify issue: firefox %d chrome %d",
			f.to(firefox), f.to(chrome))
	}
	r := f.sent[0]
	if r.Headers["Content-Encoding"] != "aes128gcm" ||
		!strings.HasPrefix(r.Headers["Authorization"], "vapid t=") {
		t.Errorf("request = %+v", r.Headers)
	}
	p.check(ctx, func() []IncidentListItem { return nil },
		testEpoch)
	if len(f.sent) != 1 {
		t.Error("a check without the pipeline sent")
	}
}

func TestPusherForgetsGoneBrowsersAndNotesFailures(t *testing.T) {
	store := openPushStore(t)
	p, f := newTestPusher(t, store)
	for _, ep := range []string{chrome, chrome2, firefox} {
		_ = p.book.subscribe(testSub(ep), pushPrefs{Min: "notify"}, "",
			testEpoch)
	}
	f.fail[firefox] = &transport.StatusError{StatusCode: 410}
	dial := &url.Error{Op: "Post", URL: "https://fcm.googleapis.com",
		Err: errors.New("dial tcp: i/o timeout")}
	f.fail[chrome], f.fail[chrome2] = dial, dial
	ctx := context.Background()
	p.check(ctx, items(), testEpoch)
	p.check(ctx, items(pushTestItem("a", "notify", "open")),
		testEpoch)
	if on, _ := p.book.status(firefox); on {
		t.Error("a browser whose subscription ended is kept")
	}
	if f.to(chrome)+f.to(chrome2) != 1 {
		t.Errorf("an unreachable service was tried %d times",
			f.to(chrome)+f.to(chrome2))
	}
	for _, ep := range []string{chrome, chrome2} {
		on, failing := p.book.status(ep)
		if !on || !strings.Contains(failing, "outbound internet") {
			t.Errorf("%s: on %v, failing %q", ep, on, failing)
		}
	}
	delete(f.fail, chrome)
	delete(f.fail, chrome2)
	p.check(ctx, items(pushTestItem("a", "notify", "open"),
		pushTestItem("b", "notify", "open")), testEpoch)
	if _, failing := p.book.status(chrome); failing != "" {
		t.Errorf("a send that went still fails: %q", failing)
	}
	again, err := loadPushBook(store, "gen1", testEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if again.key.Public() != p.book.key.Public() {
		t.Error("the key changed across a restart")
	}
	if on, _ := again.status(firefox); on {
		t.Error("the gone browser came back after a restart")
	}
	if on, _ := again.status(chrome); !on {
		t.Error("a browser was lost across a restart")
	}
}

func TestPushBookForgetsStaleBrowsers(t *testing.T) {
	store := openPushStore(t)
	p, _ := newTestPusher(t, store)
	_ = p.book.subscribe(testSub(chrome), pushPrefs{Min: "page"}, "", testEpoch)
	_ = p.book.subscribe(testSub(firefox), pushPrefs{Min: "page"}, "",
		testEpoch.Add(20*24*time.Hour))
	later := testEpoch.Add(pushSubLife + time.Hour)
	if subs := p.book.targets(later); len(subs) != 1 {
		t.Errorf("after 30 days unseen: %d browsers", len(subs))
	}
	other, err := loadPushBook(store, "gen2", testEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if subs := other.targets(testEpoch); len(subs) != 0 {
		t.Errorf("a new password kept %d browsers", len(subs))
	}
}

func TestPushBookMakesRoomAtTheLimit(t *testing.T) {
	p, _ := newTestPusher(t, openPushStore(t))
	for i := range maxPushSubs {
		ep := chrome + "-" + strings.Repeat("x", i)
		_ = p.book.subscribe(testSub(ep), pushPrefs{Min: "page"}, "",
			testEpoch.Add(time.Duration(i)*time.Minute))
	}
	_ = p.book.subscribe(testSub(firefox), pushPrefs{Min: "page"}, "",
		testEpoch.Add(time.Hour*24))
	subs := p.book.targets(testEpoch.Add(time.Hour * 24))
	if len(subs) != maxPushSubs {
		t.Fatalf("%d browsers kept", len(subs))
	}
	if _, ok := subs[pushID(chrome+"-")]; ok {
		t.Error("the browser seen longest ago was kept")
	}
	if _, ok := subs[pushID(firefox)]; !ok {
		t.Error("the new browser was not added")
	}
}

func TestPusherReadsNothingWithoutBrowsers(t *testing.T) {
	p, f := newTestPusher(t, openPushStore(t))
	ctx := context.Background()
	read := 0
	list := func() []IncidentListItem {
		read++
		return []IncidentListItem{pushTestItem("a", "notify", "open")}
	}
	p.check(ctx, list, testEpoch)
	if read != 0 {
		t.Fatal("issues read with nobody subscribed")
	}
	_ = p.book.subscribe(testSub(chrome), pushPrefs{Min: "notify"}, "",
		testEpoch)
	p.check(ctx, list, testEpoch)
	p.check(ctx, list, testEpoch)
	if read != 2 || len(f.sent) != 0 {
		t.Errorf("read %d, sent %d: what was there is not news",
			read, len(f.sent))
	}
}

func TestPusherTriesOthersAfterAnAnswerWithoutStatus(t *testing.T) {
	p, f := newTestPusher(t, openPushStore(t))
	for _, ep := range []string{chrome, chrome2} {
		_ = p.book.subscribe(testSub(ep), pushPrefs{Min: "notify"}, "",
			testEpoch)
	}
	odd := transport.Permanent(errors.New("unexpected redirect"))
	f.fail[chrome], f.fail[chrome2] = odd, odd
	ctx := context.Background()
	p.check(ctx, items(), testEpoch)
	p.check(ctx, items(pushTestItem("a", "notify", "open")), testEpoch)
	if f.to(chrome) != 1 || f.to(chrome2) != 1 {
		t.Errorf("sent %d and %d: a service that answered was skipped",
			f.to(chrome), f.to(chrome2))
	}
	if _, failing := p.book.status(chrome); strings.Contains(failing,
		"outbound internet") {
		t.Errorf("an answer was taken for no connection: %q", failing)
	}
}

func TestPushBookDropsAFailingBrowserFirstAtTheLimit(t *testing.T) {
	p, _ := newTestPusher(t, openPushStore(t))
	var failing string
	for i := range maxPushSubs {
		ep := chrome + "-" + strings.Repeat("x", i)
		_ = p.book.subscribe(testSub(ep), pushPrefs{Min: "page"}, "",
			testEpoch.Add(time.Duration(i)*time.Minute))
		if i == 50 {
			failing = ep
		}
	}
	p.book.noteSend(pushID(failing),
		&transport.StatusError{StatusCode: 500})
	_ = p.book.subscribe(testSub(firefox), pushPrefs{Min: "page"}, "",
		testEpoch.Add(24*time.Hour))
	subs := p.book.targets(testEpoch.Add(24 * time.Hour))
	if _, ok := subs[pushID(failing)]; ok {
		t.Error("the failing browser was kept")
	}
	if _, ok := subs[pushID(chrome+"-")]; !ok {
		t.Error("the oldest working browser was dropped")
	}
}
