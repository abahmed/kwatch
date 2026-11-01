package ntfy

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestResolvePriorityRules(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   float64
	}{
		{"unset gives 3", map[string]interface{}{}, 3},
		{"5 is capped to 3", map[string]interface{}{"priority": 5}, 3},
		{"1 stays 1", map[string]interface{}{"priority": 1}, 1},
		{"override beats cap", map[string]interface{}{
			"priority": 5, "resolvedPriority": 4}, 4},
		{"override beats low", map[string]interface{}{
			"priority": 1, "resolvedPriority": 2}, 2},
		{"invalid override ignored", map[string]interface{}{
			"priority": 5, "resolvedPriority": 9}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := providertest.NewRecorder(t)
			tc.config["topic"] = "kwatch"
			c := NewNtfy(tc.config, "dev", rec.Dependencies())
			c.url = rec.URL()
			err := c.SendIncident(
				context.Background(), providertest.Resolve())
			if err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			got := rec.Last(t).JSON(t)["priority"]
			if got != tc.want {
				t.Fatalf("priority = %v, want %v", got, tc.want)
			}
		})
	}
}
