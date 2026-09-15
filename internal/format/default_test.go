package format

import "testing"

func TestOrDefaultUsesFallbackOnlyForEmptyValues(t *testing.T) {
	if got := OrDefault("", "fallback"); got != "fallback" {
		t.Fatalf("OrDefault(empty) = %q", got)
	}
	if got := OrDefault("value", "fallback"); got != "value" {
		t.Fatalf("OrDefault(value) = %q", got)
	}
}
