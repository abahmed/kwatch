package providertest

import (
	"context"
	"testing"

	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/notification"
)

// IncidentSender is the part of a paging provider the notice tests drive.
type IncidentSender interface {
	SendMessage(context.Context, string) error
	SendIncident(context.Context, notification.Message) error
}

// AssertNoticesSkipped checks that a paging provider never sends a plain
// notice, which would open an alert nothing resolves, nor the startup
// summary, whose problems page on their own, while a real incident still
// reaches the recorder.
func AssertNoticesSkipped(t *testing.T, p IncidentSender, rec *Recorder) {
	t.Helper()
	skipper, ok := p.(deliveryapi.PlainMessageSkipper)
	if !ok || !skipper.SkipsPlainMessages() {
		t.Fatal("provider must declare that it skips plain messages")
	}
	ctx := context.Background()
	cases := []struct {
		name     string
		send     func() error
		requests int
	}{
		{name: "plain message is skipped", requests: 0,
			send: func() error { return p.SendMessage(ctx, "kwatch started") }},
		{name: "notice message is skipped", requests: 0,
			send: func() error {
				return p.SendIncident(ctx, notification.Notice("upgrade"))
			}},
		{name: "message without key is skipped", requests: 0,
			send: func() error {
				return p.SendIncident(ctx, notification.Message{Title: "x"})
			}},
		{name: "startup summary is skipped", requests: 0,
			send: func() error { return p.SendIncident(ctx, Summary()) }},
		{name: "startup summary resolve is skipped", requests: 0,
			send: func() error {
				m := Summary()
				m.Revision, m.Status = 2, notification.StatusResolved
				return p.SendIncident(ctx, m)
			}},
		{name: "incident is sent", requests: 1,
			send: func() error { return p.SendIncident(ctx, Announce()) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec.Reset()
			if err := tc.send(); err != nil {
				t.Fatalf("send: %v", err)
			}
			if got := len(rec.Requests()); got != tc.requests {
				t.Fatalf("requests = %d, want %d", got, tc.requests)
			}
		})
	}
}
