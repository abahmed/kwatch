package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestParseTestAlertArgs(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		ok       bool
		provider string
		sender   string
	}{
		{nil, true, "", "kwatch test-alert"},
		{[]string{"--provider", "slack"}, true, "slack",
			"kwatch test-alert"},
		{[]string{"--provider=slack", "--from=installer"}, true,
			"slack", "installer"},
		{[]string{"--provider"}, false, "", ""},
		{[]string{"--bogus", "x"}, false, "", ""},
		{[]string{"--from", "someone"}, false, "", ""},
	} {
		got, ok := parseTestAlertArgs(tc.args)
		if ok != tc.ok {
			t.Fatalf("%q: ok = %v", tc.args, ok)
		}
		if ok && (got.provider != tc.provider || got.sender != tc.sender) {
			t.Fatalf("%q: got %+v", tc.args, got)
		}
	}
}

func TestRunTestAlertUsageErrorExits64(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runTestAlert([]string{"--nope"}, &out, &errOut, time.Now)
	if code != testAlertUsage {
		t.Fatalf("exit %d", code)
	}
}

func TestDeliverTestAlertWithoutProvidersExits2(t *testing.T) {
	am := delivery.NewManagerWithDependencies(delivery.Dependencies{
		Clock: clock.RealClock{},
	})
	var out, errOut bytes.Buffer
	code := deliverTestAlert(am, "", "x", &out, &errOut)
	if code != testAlertNoTargets {
		t.Fatalf("exit %d", code)
	}
	want := "No notification provider is set up, so there is nowhere " +
		"to send a test.\nFix: run the installer and choose Edit " +
		"notification providers.\n"
	if out.String() != want {
		t.Fatalf("output %q", out.String())
	}
}

func TestTestAlertTextSaysNothingIsWrong(t *testing.T) {
	at := time.Date(2026, 10, 19, 9, 5, 0, 0, time.UTC)
	text := testAlertText("prod-eu", "installer", at)
	for _, want := range []string{
		"kwatch test alert", "This is a test from kwatch. Nothing is " +
			"wrong.", "alerts from prod-eu reach this channel.",
		"Sent 2026-10-19 09:05 UTC by installer.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q missing from %q", want, text)
		}
	}
}

func TestWriteTestResultsExitCodes(t *testing.T) {
	sent := delivery.TestResult{Provider: "slack",
		Outcome: delivery.TestSent, Elapsed: 400 * time.Millisecond}
	failed := delivery.TestResult{Provider: "discord",
		Outcome: delivery.TestFailed,
		Err:     &transport.StatusError{Provider: "discord", StatusCode: 404}}
	skipped := delivery.TestResult{Provider: "pagerduty",
		Outcome: delivery.TestSkipped, Reason: "paging"}
	for _, tc := range []struct {
		results []delivery.TestResult
		code    int
		want    string
	}{
		{[]delivery.TestResult{sent, sent}, 0, "Both providers accepted"},
		{[]delivery.TestResult{sent, failed}, testAlertFailed,
			"1 of 2 providers failed."},
		{[]delivery.TestResult{skipped}, testAlertNoTargets, "skipped"},
		{[]delivery.TestResult{sent, skipped}, 0, "1 of 2 providers"},
	} {
		var out bytes.Buffer
		if code := writeTestResults(&out, tc.results); code != tc.code {
			t.Fatalf("exit %d, want %d: %s", code, tc.code, out.String())
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%q missing from %q", tc.want, out.String())
		}
	}
}

func TestClassifyTestFailureNeverPrintsSecrets(t *testing.T) {
	status := func(code int) error {
		return fmt.Errorf("send: %w", &transport.StatusError{
			Provider: "x", StatusCode: code})
	}
	for _, tc := range []struct {
		err    error
		detail string
	}{
		{status(401), "rejected the credentials (401)"},
		{status(404), "the webhook returned 404 (not found)"},
		{status(429), "rate limited (429)"},
		{status(503), "the provider returned 503"},
		{&textproto.Error{Code: 535, Msg: "bad"},
			"the mail server refused the login (535)"},
		{context.DeadlineExceeded, "no answer after 10s"},
		{&net.DNSError{Name: "hooks.example.internal"},
			"could not reach the provider"},
		{errors.New("x509: certificate signed by unknown authority"),
			"certificate not trusted"},
	} {
		got := classifyTestFailure(tc.err, "10s")
		if got.detail != tc.detail {
			t.Fatalf("%v: detail %q, want %q", tc.err, got.detail,
				tc.detail)
		}
	}
	leak := errors.New("post https://hooks.slack.com/T/B/secret " +
		"token=abcdef123456 for ops@example.com")
	got := classifyTestFailure(leak, "10s").detail
	for _, secret := range []string{"hooks.slack.com", "abcdef123456",
		"ops@example.com"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%q left in %q", secret, got)
		}
	}
}
