package wecom

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact robot markdown request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewWecom(map[string]interface{}{
		"webhook": rec.URL() + "/cgi-bin/webhook/send?key=hk",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"hk"}, LimitBytes: wecomTextLimit,
		Check: checkMarkdown,
	})
}

func checkMarkdown(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	b := reqs[0].JSON(t)
	md, _ := b["markdown"].(map[string]any)
	text, _ := md["content"].(string)
	if b["msgtype"] != "markdown" || text == "" {
		t.Fatalf("markdown message lacks fields: %v", b)
	}
	if len(text) > wecomTextLimit {
		t.Fatalf("content is %d bytes", len(text))
	}
	providertest.AssertFencesBalanced(t, text)
}
