package core

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestInvestigatorReadsContainers(t *testing.T) {
	readCount := 0

	reader := func(ctx context.Context, id knowledge.EntityID) []string {
		readCount++
		return []string{"error"}
	}

	investigator := NewInvestigator(reader)

	c1 := knowledge.NewEntityID(kube.KindContainer, "default",
		"pod/c1")

	p := problem.Problem{
		Root: knowledge.NewEntityID(kube.KindPod, "default", "pod"),
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: c1, Reason: "C1"}.Key(): {
				Entity: c1, Reason: "C1",
				Severity: signal.Critical, Summary: "c1",
			},
		},
	}

	investigator(context.Background(), p)

	if readCount == 0 {
		t.Error("no containers investigated")
	}
}

func TestInvestigatorDedupsLines(t *testing.T) {
	callCount := 0
	reader := func(ctx context.Context,
		id knowledge.EntityID) []string {
		callCount++
		if callCount == 1 {
			return []string{"error 1", "common"}
		}
		return []string{"error 2", "common"}
	}

	investigator := NewInvestigator(reader)

	c1 := knowledge.NewEntityID(kube.KindContainer, "default",
		"pod/c1")
	c2 := knowledge.NewEntityID(kube.KindContainer, "default",
		"pod/c2")

	p := problem.Problem{
		Root: knowledge.NewEntityID(kube.KindPod, "default", "pod"),
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: c1, Reason: "C1"}.Key(): {
				Entity: c1, Reason: "C1",
				Severity: signal.Critical, Summary: "c1",
			},
			signal.Signal{Entity: c2, Reason: "C2"}.Key(): {
				Entity: c2, Reason: "C2",
				Severity: signal.Critical, Summary: "c2",
			},
		},
	}

	output := investigator(context.Background(), p)

	commonCount := 0
	for _, line := range output {
		if line == "common" {
			commonCount++
		}
	}

	if commonCount > 1 {
		t.Errorf("duplicate line found, got %d", commonCount)
	}
}
