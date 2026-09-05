package correlation

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

func TestRemoveExpiredCooldowns(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	expired := model.IncidentKey("expired")
	active := model.IncidentKey("active")
	engine := &Engine{
		cleanupCooldown: map[model.IncidentKey]time.Time{
			expired: now.Add(-time.Second),
			active:  now.Add(time.Second),
		},
		loggedSkips: map[model.IncidentKey]string{expired: "cooldown"},
	}

	engine.removeExpiredCooldowns(now)

	if _, exists := engine.cleanupCooldown[expired]; exists {
		t.Fatal("expired cooldown was not removed")
	}
	if _, exists := engine.cleanupCooldown[active]; !exists {
		t.Fatal("active cooldown was removed")
	}
	if _, exists := engine.loggedSkips[expired]; exists {
		t.Fatal("expired cooldown audit state was not removed")
	}
}
