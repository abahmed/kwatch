package core

import (
	"context"
	"sort"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

// maxInvestigatedContainers bounds log reads per announcement.
const maxInvestigatedContainers = 2

// ContainerOutput reads a container's recent output.
type ContainerOutput func(context.Context, knowledge.EntityID) []string

// NewInvestigator returns an investigation that reads the output of the
// problem's crashing containers, most severe first.
func NewInvestigator(
	read ContainerOutput,
) func(context.Context, problem.Problem) []string {
	return func(ctx context.Context, p problem.Problem) []string {
		var containers []signal.Signal
		for key, s := range p.Members {
			if key.Entity.Kind == kube.KindContainer {
				containers = append(containers, s)
			}
		}
		sort.Slice(containers, func(i, j int) bool {
			if containers[i].Severity != containers[j].Severity {
				return containers[i].Severity > containers[j].Severity
			}
			return containers[i].Entity.String() <
				containers[j].Entity.String()
		})
		var out []string
		seen := map[string]bool{}
		for i, s := range containers {
			if i >= maxInvestigatedContainers {
				break
			}
			for _, line := range read(ctx, s.Entity) {
				if !seen[line] {
					seen[line] = true
					out = append(out, line)
				}
			}
		}
		return out
	}
}
