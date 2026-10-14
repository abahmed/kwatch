package matrix

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact room-message request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	c, rec := recorderMatrix(t)
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"secret"}, LimitBytes: 16000,
		Check: checkRoomMessage,
	})
}

func checkRoomMessage(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "PUT" {
		t.Fatalf("requests = %+v", reqs)
	}
	r := reqs[0]
	const prefix = "/_matrix/client/v3/rooms/"
	if !strings.HasPrefix(r.Path, prefix) ||
		!strings.Contains(r.RawPath, "%21room:example/send/m.room.message/") {
		t.Fatalf("path = %q (%q)", r.Path, r.RawPath)
	}
	if r.Header.Get("Authorization") != "Bearer secret" {
		t.Fatal("missing bearer token")
	}
	b := r.JSON(t)
	if b["msgtype"] != "m.text" || b["body"] == "" ||
		b["format"] != "org.matrix.custom.html" ||
		b["formatted_body"] == "" {
		t.Fatalf("m.room.message lacks required fields: %v", b)
	}
	f, _ := b["formatted_body"].(string)
	if strings.Count(f, "<pre>") != strings.Count(f, "</pre>") {
		t.Fatalf("unbalanced <pre> in %.200q", f)
	}
}
