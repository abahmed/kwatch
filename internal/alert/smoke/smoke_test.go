//go:build providersmoke

package smoke

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/alert/opsgenie"
	"github.com/abahmed/kwatch/internal/alert/pagerduty"
	"github.com/abahmed/kwatch/internal/alert/slack"
	"github.com/abahmed/kwatch/internal/alert/teams"
	"github.com/abahmed/kwatch/internal/alert/webhook"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type sender interface {
	SendIncident(context.Context, notification.Message) error
}

// TestProviderSmoke opens and resolves one incident on each configured
// real account. A provider whose environment variable is unset is skipped.
func TestProviderSmoke(t *testing.T) {
	deps := transport.Dependencies{}
	cases := []struct {
		name string
		env  string
		make func(secret string) sender
	}{
		{"slack", "KWATCH_SMOKE_SLACK_WEBHOOK", func(s string) sender {
			return slack.NewSlack(map[string]interface{}{"webhook": s},
				"smoke", deps)
		}},
		{"pagerduty", "KWATCH_SMOKE_PAGERDUTY_KEY", func(s string) sender {
			return pagerduty.NewPagerDuty(
				map[string]interface{}{"integrationKey": s}, "smoke", deps)
		}},
		{"opsgenie", "KWATCH_SMOKE_OPSGENIE_KEY", func(s string) sender {
			return opsgenie.NewOpsgenie(
				map[string]interface{}{"apiKey": s}, "smoke", deps)
		}},
		{"teams", "KWATCH_SMOKE_TEAMS_WEBHOOK", func(s string) sender {
			return teams.NewTeams(map[string]interface{}{"webhook": s},
				"smoke", deps)
		}},
		{"webhook", "KWATCH_SMOKE_WEBHOOK_URL", func(s string) sender {
			return webhook.NewWebhook(map[string]interface{}{"url": s},
				"smoke", deps)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secret := os.Getenv(tc.env)
			if secret == "" {
				t.Skipf("%s is not set", tc.env)
			}
			p := tc.make(secret)
			if reflect.ValueOf(p).IsNil() {
				t.Fatal("provider rejected its config")
			}
			ctx, cancel := context.WithTimeout(context.Background(),
				30*time.Second)
			defer cancel()
			open := providertest.Announce()
			open.Key = "smoke-" + time.Now().UTC().Format("20060102T150405")
			open.Short = "kwatch provider smoke test, safe to ignore"
			open.Note = open.Short
			if err := p.SendIncident(ctx, open); err != nil {
				t.Fatalf("open: %v", err)
			}
			done := open
			done.Revision, done.Status = 2, notification.StatusResolved
			done.Marker = notification.MarkerResolved
			if err := p.SendIncident(ctx, done); err != nil {
				t.Fatalf("resolve: %v", err)
			}
		})
	}
}
