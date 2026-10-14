package pushover

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact messages and receipt-cancel requests for
// the canonical messages. Update with KWATCH_UPDATE_ALL=1 go test -run
// TestContract -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"status":1,"receipt":"rcpt1"}`))
	}
	c := NewPushover(map[string]interface{}{
		"token": "po-secret", "user": "user123", "title": "kwatch",
		"priority": 2, "retry": 60, "expire": 3600,
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/1/messages.json"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"po-secret"}, LimitBytes: 1024,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	want := 1
	if name == "resolve" {
		want = 2 // cancel the emergency receipt, then push "resolved"
	}
	if len(reqs) != want {
		t.Fatalf("%s: %d requests, want %d", name, len(reqs), want)
	}
	msg := reqs[len(reqs)-1]
	if name == "resolve" {
		cancel := reqs[0]
		if cancel.Path != "/1/receipts/rcpt1/cancel.json" {
			t.Fatalf("cancel path = %s", cancel.Path)
		}
		if form(t, cancel)["token"] != "po-secret" {
			t.Fatal("cancel lacks the app token")
		}
	}
	f := form(t, msg)
	if msg.Path != "/1/messages.json" || f["token"] != "po-secret" ||
		f["user"] != "user123" || f["message"] == "" {
		t.Fatalf("%s: message lacks required fields: %v", name, f)
	}
	if len(f["message"]) > 1024 {
		t.Fatalf("%s: message is %d bytes", name, len(f["message"]))
	}
	switch name {
	case "page-open", "markdown-code", "unicode", "near-limit":
		if f["priority"] != "2" || f["retry"] != "60" ||
			f["expire"] != "3600" {
			t.Fatalf("%s: emergency needs retry/expire: %v", name, f)
		}
	default:
		if _, ok := f["priority"]; ok {
			t.Fatalf("%s: unexpected priority %q", name, f["priority"])
		}
	}
}

func form(t *testing.T, r providertest.Request) map[string]string {
	t.Helper()
	v, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k := range v {
		out[k] = strings.Join(v[k], ",")
	}
	return out
}
