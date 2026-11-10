package logsetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

func routed(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	klog.InitFlags(fs)
	var out, errOut bytes.Buffer
	routeText(fs, &out, &errOut)
	t.Cleanup(func() {
		klog.Flush()
		klog.ClearLogger()
		klog.SetOutput(nil)
		_ = fs.Set("logtostderr", "true")
	})
	return &out, &errOut
}

func TestTextInfoAndWarningGoToStdoutErrorToStderr(t *testing.T) {
	out, errOut := routed(t)

	klog.InfoS("provider send", "n", 1)
	klog.Warning("careful")
	klog.ErrorS(errors.New("boom"), "send failed")
	klog.Flush()

	if !strings.Contains(out.String(), `] "provider send" n=1`) ||
		!strings.Contains(out.String(), "careful") ||
		strings.Contains(out.String(), "send failed") {
		t.Fatalf("stdout = %q", out.String())
	}
	if strings.Count(errOut.String(), "\n") != 1 ||
		!strings.Contains(errOut.String(), "send failed") ||
		strings.Contains(errOut.String(), "provider send") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if !strings.HasPrefix(out.String(), "I") {
		t.Fatalf("line format changed: %q", out.String())
	}
}

func decode(t *testing.T, b *bytes.Buffer) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q: %v", l, err)
		}
		rows = append(rows, m)
	}
	return rows
}

func TestJSONFieldsAndStreams(t *testing.T) {
	var out, errOut bytes.Buffer
	UseJSON(&out, &errOut)
	t.Cleanup(Reset)

	klog.InfoS("provider send", "provider", "slack")
	klog.Warning("careful")
	klog.ErrorS(errors.New("boom"), "send failed", "n", 2)

	rows := decode(t, &out)
	if len(rows) != 2 {
		t.Fatalf("stdout rows = %v", rows)
	}
	info := rows[0]
	if info["level"] != "info" || info["msg"] != "provider send" ||
		info["provider"] != "slack" {
		t.Fatalf("info = %v", info)
	}
	ts, _ := info["ts"].(string)
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Fatalf("ts = %v", info["ts"])
	}
	c, _ := info["caller"].(string)
	if !strings.HasPrefix(c, "logsetup_test.go:") {
		t.Fatalf("caller = %v", info["caller"])
	}
	if rows[1]["level"] != "warning" || rows[1]["msg"] != "careful" {
		t.Fatalf("warning = %v", rows[1])
	}
	e := decode(t, &errOut)
	if len(e) != 1 || e[0]["level"] != "error" ||
		e[0]["msg"] != "send failed" || e[0]["err"] != "boom" ||
		e[0]["n"] != float64(2) {
		t.Fatalf("stderr = %v", e)
	}
}

func TestFormatPrefersEnvironment(t *testing.T) {
	t.Setenv(EnvFormat, "json")
	if !Wants(Format("text")) {
		t.Fatal("env should win")
	}
	t.Setenv(EnvFormat, "")
	if Wants(Format("text")) || !Wants(Format(" JSON ")) {
		t.Fatal("configured value used when env is empty")
	}
}
