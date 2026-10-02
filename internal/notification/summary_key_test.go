package notification_test

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

func TestMessageIsInformational(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	writer := compose.Writer{}
	cases := []struct {
		name    string
		msg     notification.Message
		summary bool
		info    bool
	}{
		{"startup summary", writer.StartupSummary(nil, at), true, true},
		{"startup summary resolve",
			writer.StartupResolved(compose.StartupKey(at), 2), true, true},
		{"plain notice", notification.Notice("kwatch started"), false, true},
		{"incident", notification.Message{Key: "pod/web"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.IsSummary(); got != tc.summary {
				t.Fatalf("IsSummary = %v, want %v", got, tc.summary)
			}
			if got := tc.msg.IsInformational(); got != tc.info {
				t.Fatalf("IsInformational = %v, want %v", got, tc.info)
			}
		})
	}
}
