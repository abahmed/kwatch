package story

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notice"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestStartupSummarySingleProblem(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	decisions := []problem.Decision{
		{
			Problem: problem.Problem{
				ID:   "test-1",
				Root: rootID,
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
						Entity:   rootID,
						Reason:   "CrashLooping",
						Severity: signal.Critical,
						Since:    now.Add(-5 * time.Minute),
						Summary:  "Pod is crash looping",
					},
				},
			},
		},
	}

	msg := StartupSummary(decisions, now)

	if msg.Key != "startup" {
		t.Errorf("Key = %s, want 'startup'", msg.Key)
	}
	if !strings.Contains(msg.Title, "1 existing") {
		t.Errorf("Title should mention count: %s", msg.Title)
	}
	if msg.Status != notice.StatusWarning {
		t.Errorf("notice.Status = %v, want Warning", msg.Status)
	}
}

func TestStartupSummaryMultipleProblems(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	decisions := []problem.Decision{
		{
			Problem: problem.Problem{
				ID:   "test-1",
				Root: knowledge.NewEntityID("pod", "default", "api-1"),
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"default", "api-1"),
						Reason: "CrashLooping",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"default", "api-1"),
						Reason:   "CrashLooping",
						Severity: signal.Critical,
						Since:    now.Add(-5 * time.Minute),
						Summary:  "Pod is crash looping",
					},
				},
			},
		},
		{
			Problem: problem.Problem{
				ID:   "test-2",
				Root: knowledge.NewEntityID("pod", "default", "db-1"),
				Tier: problem.Page,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"default", "db-1"),
						Reason: "OOMKilled",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"default", "db-1"),
						Reason:   "OOMKilled",
						Severity: signal.Critical,
						Since:    now.Add(-10 * time.Minute),
						Summary:  "Pod was OOMKilled",
					},
				},
			},
		},
	}

	msg := StartupSummary(decisions, now)

	if !strings.Contains(msg.Title, "2 existing") {
		t.Errorf("Title should mention 2 problems: %s", msg.Title)
	}
	if len(msg.Lines) < 3 {
		t.Errorf("should have at least 3 lines (2 problems + footer)")
	}
}

func TestStartupSummarySortedByTier(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	decisions := []problem.Decision{
		{
			Problem: problem.Problem{
				ID:   "test-1",
				Root: knowledge.NewEntityID("pod", "ns1", "p1"),
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"ns1", "p1"),
						Reason: "CrashLooping",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"ns1", "p1"),
						Reason:   "CrashLooping",
						Severity: signal.Critical,
						Since:    now,
						Summary:  "Pod failing (notify tier)",
					},
				},
			},
		},
		{
			Problem: problem.Problem{
				ID:   "test-2",
				Root: knowledge.NewEntityID("pod", "ns2", "p2"),
				Tier: problem.Page,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"ns2", "p2"),
						Reason: "OOMKilled",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"ns2", "p2"),
						Reason:   "OOMKilled",
						Severity: signal.Critical,
						Since:    now,
						Summary:  "Pod failing (page tier)",
					},
				},
			},
		},
	}

	msg := StartupSummary(decisions, now)

	pageIdx := -1
	notifyIdx := -1
	for i, line := range msg.Lines {
		if strings.Contains(line, "page tier") {
			pageIdx = i
		}
		if strings.Contains(line, "notify tier") {
			notifyIdx = i
		}
	}

	if pageIdx >= 0 && notifyIdx >= 0 && pageIdx > notifyIdx {
		t.Errorf("page tier (higher) should come before notify tier")
	}
}

func TestStartupSummaryCappedAtMaxLines(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	decisions := make([]problem.Decision, maxSummaryLines+5)
	for i := 0; i < maxSummaryLines+5; i++ {
		decisions[i] = problem.Decision{
			Problem: problem.Problem{
				ID: "test-" + string(rune(i)),
				Root: knowledge.NewEntityID("pod", "ns", "p"+
					string(rune(i))),
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"+string(rune(i))),
						Reason: "Test",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"+string(rune(i))),
						Reason:   "Test",
						Severity: signal.Critical,
						Since:    now,
						Summary:  "Pod problem",
					},
				},
			},
		}
	}

	msg := StartupSummary(decisions, now)

	problemCount := 0
	for _, line := range msg.Lines {
		if strings.HasPrefix(line, "•") {
			problemCount++
		}
	}

	if problemCount > maxSummaryLines {
		t.Errorf("too many problems: got %d, max %d",
			problemCount, maxSummaryLines)
	}

	if !strings.Contains(msg.Lines[len(msg.Lines)-2],
		"…and") {
		t.Errorf("should have '…and N more' line")
	}
}

func TestStartupSummaryMessageKey(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	decisions := []problem.Decision{
		{
			Problem: problem.Problem{
				ID:   "test-1",
				Root: knowledge.NewEntityID("pod", "ns", "p"),
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"),
						Reason: "Test",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"),
						Reason:   "Test",
						Severity: signal.Critical,
						Since:    now,
						Summary:  "Pod problem",
					},
				},
			},
		},
	}

	msg := StartupSummary(decisions, now)

	if msg.Key != "startup" {
		t.Errorf("message key should be 'startup', got '%s'",
			msg.Key)
	}
}

func TestStartupSummaryIncludesContinuation(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	decisions := []problem.Decision{
		{
			Problem: problem.Problem{
				ID:   "test-1",
				Root: knowledge.NewEntityID("pod", "ns", "p"),
				Tier: problem.Notify,
				Members: map[signal.Key]signal.Signal{
					signal.Signal{
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"),
						Reason: "Test",
					}.Key(): {
						Entity: knowledge.NewEntityID("pod",
							"ns", "p"),
						Reason:   "Test",
						Severity: signal.Critical,
						Since:    now,
						Summary:  "Pod problem",
					},
				},
			},
		},
	}

	msg := StartupSummary(decisions, now)

	found := false
	for _, line := range msg.Lines {
		if strings.Contains(line,
			"New changes to these problems") {
			found = true
		}
	}
	if !found {
		t.Errorf("continuation message missing from summary")
	}
}
