package app

import "testing"

func TestFlightKeepsTextUnlessTurnedOff(t *testing.T) {
	for value, want := range map[string]bool{
		"": true, "true": true, "on": true, "false": false, "OFF": false,
		" no ": false, "0": false,
	} {
		t.Setenv(flightTextEnv, value)
		if got := flightKeepsText(); got != want {
			t.Errorf("%s=%q: keeps text = %v, want %v", flightTextEnv,
				value, got, want)
		}
	}
}
