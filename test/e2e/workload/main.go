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
		blockForever()
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
		blockForever()
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
		block := make([]byte, 1024*1024)
		// Write one byte per page so the memory is really resident.
		for i := 0; i < len(block); i += 4096 {
			block[i] = 1
		}
		blocks = append(blocks, block)
	}
	blockForever()
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
	blockForever()
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

// blockForever keeps the process alive without exiting. A bare select{} in
// main would make the Go runtime stop with "all goroutines are asleep -
// deadlock!", so healthy workloads would crash instead of staying up.
func blockForever() {
	for {
		time.Sleep(time.Hour)
	}
}
