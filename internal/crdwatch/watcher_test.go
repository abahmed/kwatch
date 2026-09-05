package crdwatch

import "testing"

func TestRestartForLateConfig(t *testing.T) {
	restarts := 0
	watcher := &Watcher{restart: func() { restarts++ }}

	if watcher.restartForLateConfig(0) {
		t.Fatal("empty initial list must not restart")
	}
	if !watcher.restartForLateConfig(1) {
		t.Fatal("late initial config must request restart")
	}
	watcher.restartForLateConfig(1)
	if restarts != 1 {
		t.Fatalf("restart must happen once, got %d", restarts)
	}
}
