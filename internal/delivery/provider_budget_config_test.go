package delivery

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestBuildProviderEntryUsesConfiguredHourlyBudget(t *testing.T) {
	factory := func(
		string, map[string]interface{}, transport.ProviderContext,
	) Provider {
		return &errorRecorderProvider{name: "slack"}
	}
	for _, budget := range []int{0, 7, config.DefaultHourlyBudget} {
		entry, err := buildProviderEntry(config.ProviderRuntime{
			Name: "slack", HourlyBudget: budget,
		}, factory, transport.ProviderContext{})
		if err != nil {
			t.Fatal(err)
		}
		if entry.hourlyBudget != budget {
			t.Fatalf("budget = %d, want %d", entry.hourlyBudget, budget)
		}
	}
}
