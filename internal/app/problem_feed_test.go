package app

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestProblemFeedEmptyWithoutSource(t *testing.T) {
	feed := &problemFeed{}
	got := feed.Snapshot()
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil list, got %#v", got)
	}
}

func TestProblemFeedBuildsSafeView(t *testing.T) {
	opened := time.Unix(100, 0)
	root := knowledge.NewEntityID("node", "", "n1")
	feed := &problemFeed{}
	feed.set(func() []problem.Problem {
		return []problem.Problem{{
			ID: "node/n1", Root: root, State: problem.Open,
			Tier: problem.Page, Opened: opened,
			Cause: &reason.Hypothesis{
				Summary: "node down", Score: 0.9,
			},
			Members: map[signal.Key]signal.Signal{{}: {}},
			Impact:  []knowledge.EntityID{root, root},
		}}
	})
	got := feed.Snapshot()
	want := health.ProblemView{
		ID: "node/n1", State: "open", Tier: "page",
		Root:  health.RootView{Kind: "node", Name: "n1"},
		Cause: "node down", Confidence: 0.9, Opened: opened,
		Members: 1, ImpactCount: 2,
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	feed.clear()
	if len(feed.Snapshot()) != 0 {
		t.Fatal("cleared feed must be empty")
	}
}

func TestProblemFeedBoundsResult(t *testing.T) {
	feed := &problemFeed{}
	feed.set(func() []problem.Problem {
		return make([]problem.Problem, health.MaxProblemViews+5)
	})
	if got := len(feed.Snapshot()); got != health.MaxProblemViews {
		t.Fatalf("got %d views", got)
	}
}
