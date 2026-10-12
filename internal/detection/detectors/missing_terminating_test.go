package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// missingFindings evaluates the missing-reference detector at at.
func missingFindings(
	m *inventory.Model, at time.Time, id inventory.EntityID,
) []detection.Finding {
	r := detection.NewRegistry(nil, Missing{})
	return r.Evaluate(m, at, id).Findings
}

func TestMissingSkipsPodsOfATerminatingNamespace(t *testing.T) {
	m, pod := podMissingSecret(nil)
	put(m, newID(kube.KindNamespace, "", "default"), t0,
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Terminating")})

	assert.Empty(t, missingFindings(m, t0, pod.ID),
		"namespace deletion removes Secrets before pods")
}
