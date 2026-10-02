package providertest

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestLifecycleMessagesShareOneKey(t *testing.T) {
	cases := Lifecycle()
	if len(cases) != 3 {
		t.Fatalf("lifecycle has %d steps", len(cases))
	}
	for _, tc := range cases {
		if tc.Message.Key != Key {
			t.Fatalf("%s key = %q", tc.Name, tc.Message.Key)
		}
		AssertOneLeadingEmoji(t, tc.Message.Note)
		AssertOneLeadingEmoji(t, tc.Message.Short)
	}
	if !cases[2].Message.Resolved() || cases[0].Message.Resolved() {
		t.Fatal("only the last step resolves")
	}
	if !strings.Contains(Hostile().Note, "<script>") {
		t.Fatal("hostile fixture lost its markup")
	}
}

func TestCountEmoji(t *testing.T) {
	if CountEmoji("🔴 a ✅ b") != 2 || CountEmoji("plain") != 0 {
		t.Fatal("emoji count is wrong")
	}
}

func TestRecorderCapturesRequests(t *testing.T) {
	rec := NewRecorder(t)
	sender := transport.NewSender(rec.Dependencies())
	_, err := sender.Send(context.Background(), transport.Request{
		Provider: "test", URL: rec.URL() + "/hook?x=1",
		Body: []byte(`{"a":1}`),
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	last := rec.Last(t)
	if last.Path != "/hook" || last.Query != "x=1" ||
		last.Method != http.MethodPost || last.JSON(t)["a"] != 1.0 {
		t.Fatalf("captured %+v", last)
	}
	if !rec.Dependencies().Now().Equal(Now) {
		t.Fatal("recorder clock is not fixed")
	}
	rec.Reset()
	if len(rec.Requests()) != 0 {
		t.Fatal("reset kept requests")
	}
}
