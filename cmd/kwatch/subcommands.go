package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func runLint(strict, check bool) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	errs := config.ValidateConfig(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		os.Exit(1)
	}
	if strict {
		if err := config.LintStrict(); err != nil {
			fmt.Fprintf(os.Stderr, "STRICT ERROR: %v\n", err)
			os.Exit(1)
		}
	}
	if check {
		am := &alert.AlertManager{}
		am.Init(cfg.Alert, &cfg.App)
		results := am.VerifyAll()
		hasErr := false
		for name, err := range results {
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: FAIL — %v\n", name, err)
				hasErr = true
			} else {
				fmt.Printf("  %s: OK\n", name)
			}
		}
		if hasErr {
			os.Exit(1)
		}
	}
	fmt.Println("config OK")
}

func runReplay() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	providers := make([]string, 0, len(cfg.Alert))
	for k := range cfg.Alert {
		providers = append(providers, k)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid event line: %v\n  %s\n", err, line)
			continue
		}
		msg := fmt.Sprintf("[replay] %s/%s %s: %s", ev.Namespace, ev.PodName, ev.Reason, ev.Events)
		fmt.Printf("would notify %v: %s\n", providers, msg)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: reading stdin: %v\n", err)
		os.Exit(1)
	}
}
