package dingtalk

import (
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact signed markdown request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewDingTalk(map[string]interface{}{
		"accessToken": "dtk-abc123", "secret": "sig-secret",
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/robot/send?access_token=%s"
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"dtk-abc123", "sig-secret"}, LimitBytes: 19000,
		Check: checkMarkdown,
	})
}

func checkMarkdown(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	q, _ := url.ParseQuery(reqs[0].Query)
	if q.Get("access_token") != "dtk-abc123" || q.Get("timestamp") == "" ||
		q.Get("sign") == "" {
		t.Fatalf("query lacks token or signature: %q", reqs[0].Query)
	}
	if len(reqs[0].Body) > dingTalkBodyLimit {
		t.Fatalf("body is %d bytes", len(reqs[0].Body))
	}
	b := reqs[0].JSON(t)
	md, _ := b["markdown"].(map[string]any)
	text, _ := md["text"].(string)
	if b["msgtype"] != "markdown" || md["title"] == "" || text == "" {
		t.Fatalf("markdown message lacks fields: %v", b)
	}
	providertest.AssertFencesBalanced(t, text)
	if name == "markdown-code" && (strings.Contains(text, "`") ||
		!strings.Contains(text, `boom \*x\*`)) {
		t.Fatalf("code must be escaped plain text, not backticks: %q", text)
	}
}
