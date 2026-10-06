package main

import (
	"testing"
	"time"
)

// The runtime image has no zoneinfo, so the binary must carry its own
// (time/tzdata, imported in main.go). CronJob spec.timeZone depends on it.
func TestEmbeddedZoneDatabaseLoads(t *testing.T) {
	t.Setenv("ZONEINFO", "/nonexistent/zoneinfo.zip")
	for _, name := range []string{"Europe/Berlin", "Asia/Kolkata"} {
		if _, err := time.LoadLocation(name); err != nil {
			t.Fatalf("LoadLocation(%q): %v", name, err)
		}
	}
}
