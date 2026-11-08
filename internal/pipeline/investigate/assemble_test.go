package investigate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

func TestAssembleJoinsEvidenceWithoutWritingIntoTheLead(t *testing.T) {
	backing := make([]incident.Fact, 1, 4)
	backing[0] = incident.Fact{Text: "lead"}
	planned := []check{{name: "a", adds: true}, {name: "b"}}
	outputs := []*checkOutput{
		{summary: Result{Evidence: []incident.Fact{{Text: "extra"}}}},
		{summary: Result{Evidence: backing}},
	}
	out := assemble(planned, outputs, 1)
	assert.Equal(t, []string{"lead", "extra"},
		[]string{out.Evidence[0].Text, out.Evidence[1].Text})
	assert.Empty(t, backing[:cap(backing)][1].Text)
}
