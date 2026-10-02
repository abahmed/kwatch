package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestLogSignatureRemovesWhatDiffersBetweenReplicas(t *testing.T) {
	tests := []struct{ a, b string }{
		{"2026-09-29T10:00:01.123Z ERROR request 4f1c2a9e-0b7d-4c3e-9a51-" +
			"2d3c4b5a6f70 failed",
			"2026-09-29T11:42:59Z ERROR request 0a1b2c3d-4e5f-6a7b-8c9d-" +
				"0e1f2a3b4c5d failed"},
		{"panic: nil pointer at 0xc000123abc",
			"panic: nil pointer at 0xc000ffee00"},
		{"dial tcp 10.0.3.7:5432: connect: connection refused",
			"dial tcp 10.0.9.12:5432: connect: connection refused"},
		{"worker api-7f9c6d8b5-x2k4p crashed after 31 retries",
			"worker api-7f9c6d8b5-q8z1m crashed after 7 retries"},
		{"10:00:01 fatal: trace abcdef0123456789 aborted",
			"23:59:59 fatal: trace 0123456789abcdef aborted"},
	}
	for _, tt := range tests {
		if a, b := logSignature(tt.a), logSignature(tt.b); a != b {
			t.Errorf("signatures differ:\n%q\n%q", a, b)
		}
	}
	if logSignature("disk full") == logSignature("permission denied") {
		t.Fatal("different errors must keep different signatures")
	}
}

func TestFirstErrorLineSkipsStackFrames(t *testing.T) {
	tests := map[string][]string{
		"panic: runtime error: invalid memory address": {
			"starting server",
			"panic: runtime error: invalid memory address",
			"goroutine 1 [running]:",
			"main.main()",
			"/app/main.go:42 +0x1d",
		},
		`Exception in thread "main" java.lang.IllegalStateException: boom`: {
			`Exception in thread "main" java.lang.IllegalStateException: boom`,
			"at com.shop.Api.start(Api.java:12)",
			"at com.shop.Main.main(Main.java:3)",
		},
		"ValueError: invalid literal for int()": {
			"Traceback (most recent call last):",
			`File "/app/main.py", line 3, in <module>`,
			"int(value)",
			"ValueError: invalid literal for int()",
		},
	}
	for want, lines := range tests {
		if got := firstErrorLine(lines); got != want {
			t.Errorf("first error = %q, want %q", got, want)
		}
	}
	if got := firstErrorLine([]string{"listening on :8080"}); got != "" {
		t.Fatalf("no error line, got %q", got)
	}
}

func TestInvestigateCrashShowsReplicaSignatureOnce(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	a := kube.ContainerID("shop", "api-7f9c6d8b5-x2k4p", "app")
	b := kube.ContainerID("shop", "api-7f9c6d8b5-q8z1m", "app")
	model := testModel(t,
		observed(a, map[string]inventory.Value{
			kube.AttrLastMessage: text("cannot open /etc/app/config.yaml")}),
		observed(b, map[string]inventory.Value{
			kube.AttrLastMessage: text("cannot open /etc/app/config.yaml")}))
	logs := map[inventory.EntityID][]string{
		a: {"2026-09-29T10:00:01Z boot", "2026-09-29T10:00:02Z ERROR " +
			"db 10.0.0.4:5432 refused", "at db.connect(db.js:10)"},
		b: {"2026-09-29T10:00:07Z boot", "2026-09-29T10:00:09Z ERROR " +
			"db 10.0.0.9:5432 refused", "at db.connect(db.js:10)"},
	}
	read := func(_ context.Context, id inventory.EntityID) []string {
		return logs[id]
	}
	p := incidentOf(root, finding(a, "CrashLoop"), finding(b, "CrashLoop"))

	plan, r := planAndRun(t, Sources{Model: model, Logs: read}, p)

	if plan.Kind != kindCrash {
		t.Fatalf("kind = %q, want crash", plan.Kind)
	}
	// Containers are read in name order; addresses are redacted.
	errs := evidenceOf(r, incident.FactError)
	if len(errs) != 1 || errs[0].Text != "2026-09-29T10:00:09Z ERROR "+
		"db [private-address]:5432 refused" {
		t.Fatalf("errors = %+v, want one replica's line once", errs)
	}
	if got := evidenceOf(r, incident.FactTermination); len(got) != 1 {
		t.Fatalf("termination = %+v, want one message", got)
	}
	if len(r.Output) != 1 {
		t.Fatalf("output = %q, want the shared error line once", r.Output)
	}
}

func TestInvestigateCrashStopsWhenBudgetEnds(t *testing.T) {
	a := kube.ContainerID("shop", "api-1", "app")
	b := kube.ContainerID("shop", "api-2", "app")
	ctx, cancel := context.WithCancel(context.Background())
	reads := 0
	read := func(context.Context, inventory.EntityID) []string {
		reads++
		cancel()
		return []string{"fatal: out of disk"}
	}
	p := incidentOf(a, finding(a, "CrashLoop"), finding(b, "CrashLoop"))

	r := readCrash(ctx, Sources{Model: testModel(t), Logs: read}, p)

	if reads != 1 || len(evidenceOf(r, incident.FactError)) != 1 {
		t.Fatalf("reads = %d, evidence = %+v: want what the first read "+
			"found and no further reads", reads, r.Evidence)
	}
}

func TestBoundedCapsAndRedactsResults(t *testing.T) {
	var r Result
	for range maxEvidence + 3 {
		r.Evidence = append(r.Evidence, incident.Fact{
			Kind: incident.FactError, Text: "password=hunter2 " +
				string(make([]byte, 2*maxEvidenceText))})
	}
	r.Evidence = append(r.Evidence, incident.Fact{Kind: "empty"})
	for range maxOutputLines + 3 {
		r.Output = append(r.Output, "line")
	}

	got := bounded(r)

	if len(got.Evidence) != maxEvidence || len(got.Output) != maxOutputLines {
		t.Fatalf("sizes = %d/%d", len(got.Evidence), len(got.Output))
	}
	for _, e := range got.Evidence {
		if len(e.Text) > maxEvidenceText+len("…") {
			t.Fatalf("text is %d bytes", len(e.Text))
		}
		if strings.Contains(e.Text, "hunter2") {
			t.Fatalf("text %q must be redacted", e.Text)
		}
	}
}
