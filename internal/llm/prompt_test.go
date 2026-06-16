package llm

import (
	"strings"
	"testing"
)

func TestTailChars(t *testing.T) {
	if got := tailChars("a\nbb\nccc", 4); got != "ccc" {
		t.Errorf("tail snap: %q", got)
	}
	if got := tailChars(strings.Repeat("x", 100), 10); len(got) > 10 {
		t.Errorf("no newline must be ≤max: %d", len(got))
	}
	if got := tailChars("short", 100); got != "short" {
		t.Errorf("under max must be unchanged: %q", got)
	}
}

func TestSelectRelevant(t *testing.T) {
	small := "line1\nline2"
	if selectRelevant(small, 6000) != small {
		t.Error("under-budget must be unchanged")
	}

	big := "panic: boom\n" + strings.Repeat("noise line\n", 2000) + "final error\n"
	out := selectRelevant(big, 200)
	if len(out) > 200 {
		t.Errorf("over budget: %d", len(out))
	}
	if !strings.Contains(out, "panic") || !strings.Contains(out, "final error") {
		t.Error("must keep signal head + tail")
	}

	one := strings.Repeat("z", 5000)
	if len(selectRelevant(one, 200)) > 200 {
		t.Error("pathological line must be capped")
	}

	if strings.TrimSpace(selectRelevant("   \n  ", 4)) != "" {
		t.Error("whitespace-only → empty")
	}
}

func TestSelectRelevantEmpty(t *testing.T) {
	if got := selectRelevant("", 100); got != "" {
		t.Errorf("empty input: got %q", got)
	}
}

func TestSelectRelevantExactFit(t *testing.T) {
	s := "exactly fits"
	if got := selectRelevant(s, len(s)); got != s {
		t.Errorf("exact fit changed: got %q", got)
	}
}
