package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/version"
)

// Exit codes of `kwatch test-alert`.
const (
	testAlertFailed    = 1
	testAlertNoTargets = 2
	testAlertUsage     = 64
)

const (
	testAlertTimeout = 10 * time.Second
	testAlertTotal   = 15 * time.Second
)

type testAlertArgs struct {
	provider string
	// sender is who asked for the test: the installer passes
	// --from installer.
	sender string
}

func parseTestAlertArgs(args []string) (testAlertArgs, bool) {
	parsed := testAlertArgs{sender: "kwatch test-alert"}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if !inline {
			if i+1 >= len(args) {
				return parsed, false
			}
			i++
			value = args[i]
		}
		switch {
		case name == "--provider" && value != "":
			parsed.provider = value
		case name == "--from" && value == "installer":
			parsed.sender = "installer"
		default:
			return parsed, false
		}
	}
	return parsed, true
}

// runTestAlert sends one test message to every configured provider (or
// the one named) through the live configuration and transport.
func runTestAlert(
	args []string, out, errOut io.Writer, now func() time.Time,
) int {
	parsed, ok := parseTestAlertArgs(args)
	if !ok {
		_, _ = fmt.Fprintf(errOut, "test-alert: unexpected arguments "+
			"%q (usage: test-alert [--provider <name>])\n", args)
		return testAlertUsage
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: %v\n", err)
		return testAlertFailed
	}
	edition := currentEdition(now)
	am, code := constructProviders(cfg, edition, errOut)
	if code != 0 {
		return testAlertFailed
	}
	if sends := edition.CommunitySends(cfg); sends != "" {
		_, _ = fmt.Fprintf(out, "The Community edition sends to "+
			"%s; the inactive providers get no alerts and no test.\n\n",
			sends)
	}
	text := testAlertText(cfg.App.ClusterName, parsed.sender, now())
	return deliverTestAlert(am, parsed.provider, text, out, errOut)
}

func deliverTestAlert(
	am *delivery.Manager, only, text string, out, errOut io.Writer,
) int {
	names := am.ProviderNames()
	if len(names) == 0 {
		_, _ = fmt.Fprint(out, "No notification provider is set up, "+
			"so there is nowhere to send a test.\nFix: run the "+
			"installer and choose Edit notification providers.\n")
		return testAlertNoTargets
	}
	if only == "?" {
		// The installer reads this to offer a provider picker.
		for _, name := range names {
			_, _ = fmt.Fprintln(out, name)
		}
		return 0
	}
	if only != "" && !contains(names, only) {
		_, _ = fmt.Fprintf(errOut, "test-alert: no provider named %q "+
			"(configured: %s)\n", only, strings.Join(names, ", "))
		return testAlertUsage
	}
	ctx, cancel := context.WithTimeout(
		context.Background(), testAlertTotal)
	defer cancel()
	count := len(names)
	if only != "" {
		count = 1
	}
	_, _ = fmt.Fprintf(out, "Sending a test alert to %s.\n\n",
		plural(count, "provider"))
	results := am.SendTestAlert(ctx, only, text, testAlertTimeout)
	return writeTestResults(out, results)
}

func testAlertText(cluster, sender string, at time.Time) string {
	if cluster == "" {
		cluster = "this cluster"
	}
	return notification.MarkerNotify + " kwatch test alert\n\n" +
		"This is a test from kwatch. Nothing is wrong.\n" +
		"If you can read this, alerts from " + cluster +
		" reach this channel.\n" +
		"Sent " + at.UTC().Format("2006-01-02 15:04") + " UTC by " +
		sender + ". Version " + version.Short() + "."
}

func writeTestResults(out io.Writer, results []delivery.TestResult) int {
	width := 0
	for _, r := range results {
		width = max(width, len(r.Provider))
	}
	sent, failed := 0, 0
	for _, r := range results {
		writeTestResult(out, r, width)
		switch r.Outcome {
		case delivery.TestSent:
			sent++
		case delivery.TestFailed:
			failed++
		}
	}
	_, _ = fmt.Fprintln(out)
	return writeTestSummary(out, sent, failed, len(results))
}

func writeTestResult(out io.Writer, r delivery.TestResult, width int) {
	detail, fix := r.Reason, ""
	switch r.Outcome {
	case delivery.TestSent:
		detail = fmt.Sprintf("accepted in %.1fs", r.Elapsed.Seconds())
	case delivery.TestFailed:
		f := classifyTestFailure(r.Err, testAlertTimeout.String())
		detail, fix = f.detail, f.fix
	}
	_, _ = fmt.Fprintf(out, "  %-*s  %-7s %s\n",
		width, r.Provider, r.Outcome, detail)
	if fix != "" {
		_, _ = fmt.Fprintf(out, "  %-*s  %-7s %s\n", width, "", "", fix)
	}
}

func writeTestSummary(out io.Writer, sent, failed, total int) int {
	switch {
	case failed > 0:
		_, _ = fmt.Fprintf(out, "%d of %s failed.\n",
			failed, plural(total, "provider"))
		return testAlertFailed
	case sent == 0:
		_, _ = fmt.Fprintln(out, "No provider could take a test "+
			"message; see the reasons above.")
		return testAlertNoTargets
	case sent == 1 && total == 1:
		_, _ = fmt.Fprintln(out, "The provider accepted the message. "+
			"Check the channel to confirm it arrived.")
	case sent == total:
		lead := fmt.Sprintf("All %d", total)
		if total == 2 {
			lead = "Both"
		}
		_, _ = fmt.Fprintf(out, "%s providers accepted the message. "+
			"Check the channels to confirm it arrived.\n", lead)
	default:
		_, _ = fmt.Fprintf(out, "%d of %s accepted the message. "+
			"Check the channels to confirm it arrived.\n",
			sent, plural(total, "provider"))
	}
	return 0
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
