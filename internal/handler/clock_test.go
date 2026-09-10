package handler

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
)

func TestSetClockSharesClockWithOomTracker(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.OomMonitor.Enabled = true
	h := NewHandler(nil, cfg, nil, nil)
	want := time.Date(2026, time.September, 10, 2, 30, 0, 0, time.UTC)
	now := func() time.Time { return want }

	h.SetClock(now)

	if got := h.now(); !got.Equal(want) {
		t.Fatalf("handler clock = %v, want %v", got, want)
	}
	if got := h.oomTracker.now(); !got.Equal(want) {
		t.Fatalf("OOM tracker clock = %v, want %v", got, want)
	}
}
