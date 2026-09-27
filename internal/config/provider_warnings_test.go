package config

import (
	"strings"
	"testing"
)

func TestWarningsFlagDiscontinuedLineNotify(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"line": {"token": "x"},
	}
	found := false
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "LINE Notify") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a LINE Notify warning")
	}
}
