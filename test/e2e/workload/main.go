package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	command := "healthy"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "healthy", "sleep":
		select {}
	case "startup-error", "crash", "recurring-error":
		os.Exit(42)
	case "delayed-error":
		delayedExit()
	case "one-shot-error":
		oneShotExit()
	case "memory":
		consumeMemory()
	case "disk":
		consumeDisk()
	case "not-ready":
		serve(false)
	case "http":
		serve(true)
	default:
		fmt.Fprintf(os.Stderr, "unknown workload command %q\n", command)
		os.Exit(2)
	}
}

func delayedExit() {
	seconds := envInt("FAIL_AFTER_SECONDS", 10)
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	<-timer.C
	os.Exit(42)
}

func oneShotExit() {
	path := os.Getenv("STATE_FILE")
	if path == "" {
		path = "/state/failed"
	}
	if _, err := os.Stat(path); err == nil {
		select {}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		os.Exit(43)
	}
	if err := os.WriteFile(path, []byte("failed\n"), 0o644); err != nil {
		os.Exit(43)
	}
	os.Exit(42)
}

func consumeMemory() {
	megabytes := envInt("MEMORY_MB", 512)
	blocks := make([][]byte, 0, megabytes)
	for range megabytes {
		blocks = append(blocks, make([]byte, 1024*1024))
	}
	select {}
}

func consumeDisk() {
	path := os.Getenv("DISK_PATH")
	if path == "" {
		path = "/tmp/kwatch-e2e-disk"
	}
	file, err := os.Create(path)
	if err != nil {
		os.Exit(42)
	}
	block := make([]byte, 1024*1024)
	for range 32 {
		if _, err := file.Write(block); err != nil {
			_ = file.Close()
			os.Exit(42)
		}
	}
	select {}
}

func serve(healthy bool) {
	ready := func(w http.ResponseWriter, _ *http.Request) {
		if !healthy {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
	http.HandleFunc("/ready", ready)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		os.Exit(44)
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
