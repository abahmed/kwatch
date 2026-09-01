package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func runLint(strict, check bool, out, errOut io.Writer) int {
	cfg, err := config.LoadConfig()
	if err != nil {
		if _, writeErr := fmt.Fprintf(errOut, "ERROR: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	errs := config.ValidateConfig(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			if _, writeErr := fmt.Fprintf(errOut, "  %s\n", e); writeErr != nil {
				return 1
			}
		}
		return 1
	}
	if strict {
		if err := config.LintStrict(); err != nil {
			if _, writeErr := fmt.Fprintf(
				errOut, "STRICT ERROR: %v\n", err,
			); writeErr != nil {
				return 1
			}
			return 1
		}
	}
	if check {
		am := &alert.AlertManager{}
		am.Init(cfg.Alert, &cfg.App)
		results := am.VerifyAll()
		hasErr := false
		names := make([]string, 0, len(results))
		for name := range results {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			err := results[name]
			if err != nil {
				if _, writeErr := fmt.Fprintf(
					errOut, "  %s: FAIL — %v\n", name, err,
				); writeErr != nil {
					return 1
				}
				hasErr = true
			} else {
				if _, writeErr := fmt.Fprintf(out, "  %s: OK\n", name); writeErr != nil {
					return 1
				}
			}
		}
		if hasErr {
			return 1
		}
	}
	if _, err := fmt.Fprintln(out, "config OK"); err != nil {
		return 1
	}
	return 0
}

func runReplay(dryRun bool, in io.Reader, out, errOut io.Writer) int {
	cfg, err := config.LoadConfig()
	if err != nil {
		if _, writeErr := fmt.Fprintf(errOut, "ERROR: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	providers := make([]string, 0, len(cfg.Alert))
	for k := range cfg.Alert {
		providers = append(providers, k)
	}
	sort.Strings(providers)
	am := &alert.AlertManager{}
	am.Init(cfg.Alert, &cfg.App)
	am.SetSilences(cfg.Silences)
	am.SetTemplates(cfg.Templates)

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			if _, writeErr := fmt.Fprintf(
				errOut, "ERROR: invalid event line: %v\n  %s\n", err, line,
			); writeErr != nil {
				return 1
			}
			continue
		}
		msg := fmt.Sprintf(
			"[replay] %s/%s %s: %s",
			ev.Namespace, ev.PodName, ev.Reason, ev.Events,
		)
		if dryRun {
			if _, writeErr := fmt.Fprintf(
				out, "would replay to %v: %s\n", providers, msg,
			); writeErr != nil {
				return 1
			}
			continue
		}
		am.NotifyEvent(ev)
		if _, writeErr := fmt.Fprintf(
			out, "replayed to %v: %s\n", providers, msg,
		); writeErr != nil {
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		if _, writeErr := fmt.Fprintf(
			errOut, "ERROR: reading stdin: %v\n", err,
		); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}
