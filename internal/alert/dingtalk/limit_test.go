package dingtalk

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func newRecorded(t *testing.T) (*DingTalk, *providertest.Recorder) {
	rec := providertest.NewRecorder(t)
	c := NewDingTalk(map[string]interface{}{
		"accessToken": "tok"}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/robot/send?access_token=%s"
	return c, rec
}

// hugeIncident has a note and output far over the body limit, with
// characters that JSON escaping makes longer.
func hugeIncident() notification.Message {
	m := providertest.Announce()
	m.Note = strings.Repeat("payments <b> keeps crashing. ", 2000)
	m.Doc = nil
	m.Output = []string{strings.Repeat("panic: <oom> \"x\"\n", 4000)}
	return m
}

func TestIncidentBodyIsCutToDingTalkLimit(t *testing.T) {
	c, rec := newRecorded(t)
	if err := c.SendIncident(t.Context(), hugeIncident()); err != nil {
		t.Fatal(err)
	}
	r := rec.Requests()[0]
	if len(r.Body) > dingTalkBodyLimit {
		t.Fatalf("body is %d bytes, limit %d", len(r.Body),
			dingTalkBodyLimit)
	}
	md, _ := r.JSON(t)["markdown"].(map[string]any)
	text, _ := md["text"].(string)
	if text == "" {
		t.Fatal("text was dropped, not cut")
	}
	providertest.AssertFencesBalanced(t, text)
}

func TestPlainMessageIsCutToDingTalkLimit(t *testing.T) {
	c, rec := newRecorded(t)
	long := strings.Repeat("<", dingTalkBodyLimit)
	if err := c.SendMessage(t.Context(), long); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()[0].Body); n > dingTalkBodyLimit {
		t.Fatalf("body is %d bytes", n)
	}
}

func TestPlainMessageMentionIsBroken(t *testing.T) {
	c, rec := newRecorded(t)
	if err := c.SendMessage(t.Context(), "hi @all @13800000000"); err != nil {
		t.Fatal(err)
	}
	providertest.AssertNoMassMention(t, rec.Requests())
	if strings.Contains(string(rec.Requests()[0].Body), "@138") {
		t.Fatal("phone-number mention is live")
	}
}
