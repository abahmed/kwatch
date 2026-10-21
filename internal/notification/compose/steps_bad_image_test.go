package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A release whose image cannot be pulled is read in the pod's events,
// where the registry's answer is; the rollout state only says "waiting".
func TestBadImageReleaseOpensWithTheEventsOfThePod(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	ct := inventory.CoreID(kube.KindContainer, "shop", "api-7d9f-a/app")
	p := incident.Incident{Root: api, Cause: &rootcause.CauseRecord{
		Root: api, RollbackRevision: "4",
		Change: &inventory.Change{Entity: api}}}

	steps := nextSteps(p, []detection.Finding{{Entity: ct,
		Reason: reasons.ImagePullBackOff}})

	var commands []string
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	assert.Equal(t, []string{
		"kubectl describe pod api-7d9f-a -n shop",
		"kubectl rollout undo deployment/api -n shop --to-revision=4",
	}, commands)
}

func TestReleaseThatCrashesStillOpensWithTheRolloutState(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	ct := inventory.CoreID(kube.KindContainer, "shop", "api-7d9f-a/app")
	p := incident.Incident{Root: api, Cause: &rootcause.CauseRecord{
		Root: api, Change: &inventory.Change{Entity: api}}}

	steps := nextSteps(p, []detection.Finding{{Entity: ct,
		Reason: reasons.CrashLoopBackOff}})

	assert.Equal(t, "kubectl rollout status deployment/api -n shop",
		steps[0].Command)
}
