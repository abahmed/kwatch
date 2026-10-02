package opsgenie

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestOpsgenieConfigValidation(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	cases := []struct {
		name   string
		config map[string]interface{}
		url    string
	}{
		{"missing key", map[string]interface{}{}, ""},
		{"invalid region", map[string]interface{}{
			"apiKey": "k", "region": "mars"}, ""},
		{"default region", map[string]interface{}{"apiKey": "k"},
			opsgenieAPIURL},
		{"eu region", map[string]interface{}{
			"apiKey": "k", "region": "eu"}, opsgenieEUAPIURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewOpsgenie(tc.config, "dev", deps)
			if tc.url == "" {
				if c != nil {
					t.Fatal("expected nil provider")
				}
				return
			}
			if c == nil || c.url != tc.url || c.Name() != "Opsgenie" {
				t.Fatalf("provider = %+v", c)
			}
		})
	}
}
