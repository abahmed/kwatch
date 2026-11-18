package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// stepsIncident is a Service whose two webhook configurations fail; a
// Secret that was once a member has moved to another issue.
func stepsIncident(steps ...incident.FixStep) incident.Incident {
	p := holderIncident(objID("service", "ops", "hook-svc"),
		detection.ModeNoEndpoints,
		objID("mutatingwebhookconfiguration", "", "hook-a"),
		objID("validatingwebhookconfiguration", "", "hook-b"))
	p.Steps = steps
	return p
}

// A stored step that describes an object no longer in the incident is
// dropped; with none left the root is described.
func TestStepNamingAnObjectOutsideTheIncidentIsDropped(t *testing.T) {
	p := stepsIncident(incident.FixStep{Text: "See the object's state",
		Command: "kubectl describe secret moved-out -n ops"})

	d := toDetail(p)

	assert.Equal(t, []FixStep{{Text: rootStepText,
		Command: "kubectl -n ops describe service hook-svc"}}, d.FixSteps)
	assert.Equal(t, "kubectl -n ops describe service hook-svc",
		d.Summary.Next.Command)
}

// Steps about members, steps naming no single object and short kind
// names are kept as they are.
func TestStepsAboutTheIncidentAreKept(t *testing.T) {
	steps := []incident.FixStep{
		{Text: "a", Command: "kubectl describe " +
			"validatingwebhookconfiguration hook-b"},
		{Text: "b", Command: "kubectl -n ops describe service hook-svc"},
		{Text: "c", Command: "kubectl get pv gone-volume"},
		{Text: "d", Command: "kubectl describe pvc data -n ops"},
		{Text: "e", Command: "kubectl get events -A"},
		{Text: "f"},
	}
	p := stepsIncident(steps...)

	d := toDetail(p)

	assert.Len(t, d.FixSteps, len(steps))
	_, ok := describedObject("kubectl describe deploy/web -n ops")
	assert.False(t, ok)
}
