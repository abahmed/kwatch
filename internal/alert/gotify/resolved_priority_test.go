package gotify

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestResolvePriorityRules(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   interface{}
	}{
		{"unset stays omitted", map[string]interface{}{}, nil},
		{"high is capped", map[string]interface{}{"priority": 8}, 5.0},
		{"low stays low", map[string]interface{}{"priority": 2}, 2.0},
		{"override wins over cap", map[string]interface{}{
			"priority": 8, "resolvedPriority": 7}, 7.0},
		{"override wins over low", map[string]interface{}{
			"priority": 2, "resolvedPriority": 4}, 4.0},
		{"override without priority", map[string]interface{}{
			"resolvedPriority": 3}, 3.0},
		{"explicit zero is sent", map[string]interface{}{
			"priority": 8, "resolvedPriority": 0}, 0.0},
		{"explicit zero without priority", map[string]interface{}{
			"resolvedPriority": 0}, 0.0},
		{"invalid override ignored", map[string]interface{}{
			"priority": 8, "resolvedPriority": 11}, 5.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := providertest.NewRecorder(t)
			tc.config["url"] = rec.URL()
			tc.config["token"] = "t"
			c := NewGotify(tc.config, "dev", rec.Dependencies())
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
