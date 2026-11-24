package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSampledErrorLineReadsTheLevel(t *testing.T) {
	tt := []struct {
		name string
		line string
		want bool
	}{
		{"logfmt_info_null_error",
			`level=info msg="reconcile ok" error=null`, false},
		{"logfmt_info_denied",
			`ts=1 level=info msg="request denied by policy"`, false},
		{"logfmt_lvl_debug", `lvl=debug msg="dial timeout 5s"`, false},
		{"logfmt_warn", `level=warn msg="retry failed once"`, false},
		{"logfmt_error", `level=error msg="reconcile ok"`, true},
		{"logfmt_severity_critical", `severity=CRITICAL msg=x`, true},
		{"json_info_error_key",
			`{"level":"info","msg":"done","error":"none yet"}`, false},
		{"json_info_failed",
			`{"level":"info","msg":"webhook failed open"}`, false},
		{"json_error", `{"level":"error","msg":"boom"}`, true},
		{"json_severity_fatal", `{"severity":"FATAL","msg":"x"}`, true},
		{"klog_info", `I0102 15:04:05.1 1 x.go:1] denied: forbidden`, false},
		{"klog_warning", `W0102 15:04:05.1 1 x.go:1] failed to x`, false},
		{"klog_error", `E0102 15:04:05.1 1 x.go:1] ok`, true},
		{"klog_fatal", `F0102 15:04:05.1 1 x.go:1] ok`, true},
		{"token_info", `INFO request timeout configured`, false},
		{"token_debug_bracket", `[DEBUG] cannot find cache, refilling`, false},
		{"token_after_timestamp",
			`2024-01-02T15:04:05Z INFO connection refused earlier`, false},
		{"token_error", `ERROR something happened`, true},
		{"token_bracket_error", `ts [ERROR] x`, true},
		{"null_error_logfmt", `msg="reconcile ok" error=null`, false},
		{"nil_error_go", `msg="done" error=<nil>`, false},
		{"nil_err", `msg="done" err=nil`, false},
		{"json_null_error", `{"msg":"done","error":null}`, false},
		{"json_empty_error", `{"msg":"done","error":""}`, false},
		{"unleveled_error", `error: pq: connection refused`, true},
		{"unleveled_exception", `java.lang.IllegalStateException: x`, true},
		{"unleveled_json_error", `{"msg":"x","error":"boom"}`, true},
		{"unleveled_panic", `panic: runtime error: nil deref`, true},
		{"unknown_level_falls_back", `level=weird msg="x failed"`, true},
		{"plain_line", `served GET /healthz 200`, false},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsSampledErrorLine(tc.line))
		})
	}
}
