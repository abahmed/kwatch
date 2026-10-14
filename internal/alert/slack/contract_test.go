package slack

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Web API requests, in order, for the
// canonical messages: the root post, thread replies and the chat.update
// of the root. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	s, rec := tokenSlack(t)
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: s.SendIncident, Rec: rec, LimitBytes: maxSectionTextChars,
		Secrets: []string{"xoxb-test"},
		Check:   checkShape,
	})
}

// checkShape asserts the Web API calls: every post names the channel,
// carries fallback text and valid blocks within Slack's limits, replies
// use the root's ts, and the root edit targets that ts.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" && len(reqs) != 1 {
		t.Fatalf("a digest is one top-level post: %d", len(reqs))
	}
	for _, r := range reqs {
		form, err := url.ParseQuery(string(r.Body))
		if err != nil {
			t.Fatal(err)
		}
		method := strings.TrimPrefix(r.Path, "/")
		if r.Method != "POST" || form.Get("channel") != "C1" {
			t.Fatalf("%s %s form = %v", r.Method, r.Path, form)
		}
		switch method {
		case "chat.postMessage":
			if form.Get("text") == "" {
				t.Fatal("post lacks fallback text")
			}
			if form.Has("thread_ts") != (name == "notify-update" ||
				name == "resolve") {
				t.Fatalf("thread_ts = %q in %s", form.Get("thread_ts"),
					name)
			}
			if form.Has("thread_ts") && form.Get("thread_ts") != "111.1" {
				t.Fatalf("reply goes to %q", form.Get("thread_ts"))
			}
		case "chat.update":
			if form.Get("ts") != "111.1" {
				t.Fatalf("root edit targets %q", form.Get("ts"))
			}
		default:
			t.Fatalf("unexpected call %s", r.Path)
		}
		checkBlocks(t, form.Get("blocks"))
	}
}

func checkBlocks(t *testing.T, raw string) {
	var blocks []struct {
		Type string `json:"type"`
		Text *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatalf("blocks are not JSON: %v", err)
	}
	if len(blocks) == 0 || len(blocks) > maxBlocksPerMessage {
		t.Fatalf("%d blocks", len(blocks))
	}
	for _, b := range blocks {
		if b.Type != "section" || b.Text == nil || b.Text.Text == "" {
			continue
		}
		if b.Text.Type != "mrkdwn" && b.Text.Type != "plain_text" {
			t.Fatalf("text type %q", b.Text.Type)
		}
		if n := utf8.RuneCountInString(b.Text.Text); n >
			maxSectionTextChars {
			t.Fatalf("section is %d chars", n)
		}
		providertest.AssertFencesBalanced(t, b.Text.Text)
	}
}
