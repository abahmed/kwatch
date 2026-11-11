package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// stepChecks map a read-only command to the check that reads the same
// state. The longer prefix comes first. Every read-only step compose
// writes must map to one (TestStepChecksCoverReadOnlySteps), and every
// check named here must be registered in investigate.
var stepChecks = []struct{ prefix, check string }{
	{"kubectl get mutatingwebhookconfiguration", incident.CheckAdmission},
	{"kubectl get validatingwebhookconfiguration", incident.CheckAdmission},
	{"kubectl get ingress,pods", incident.CheckConfig},
	{"kubectl get configmap ", incident.CheckConfig},
	{"kubectl get ingress", incident.CheckIngress},
	{"kubectl get --raw ", incident.CheckControlPlane},
	{"kubectl get nodes ", incident.CheckNode},
	{"kubectl get pods -A ", incident.CheckNodePods},
	{"kubectl get pods ", incident.CheckPodState},
	{"kubectl -n kube-system ", incident.CheckControlPlane},
	{"kubectl get ", incident.CheckFinalizer},
	{"kubectl describe pod ", incident.CheckPodState},
	{"kubectl describe pods ", incident.CheckPodState},
	{"kubectl describe node ", incident.CheckNode},
	{"kubectl describe pvc ", incident.CheckVolume},
	{"kubectl describe secret ", incident.CheckConfig},
	{"kubectl describe service ", incident.CheckService},
	{"kubectl describe ", incident.CheckObject},
	{"kubectl logs ", incident.CheckLogs},
	{"kubectl top pod ", incident.CheckUsage},
	{"kubectl rollout status ", incident.CheckRollout},
}

// stepCheck is the check that covers a read-only command, or "".
func stepCheck(command string) string {
	for _, c := range stepChecks {
		if strings.HasPrefix(command, c.prefix) {
			return c.check
		}
	}
	return ""
}

// gatheredSteps drops the read-only commands whose check came back ok:
// kwatch already read what they show, and the report has it. Fix steps,
// steps without a command and uncovered commands stay. Without a report
// (Community, or no investigation) every step stays.
func gatheredSteps(
	steps []notification.Step, gathered map[string]bool,
) []notification.Step {
	if len(gathered) == 0 {
		return steps
	}
	var out []notification.Step
	for _, s := range steps {
		if !s.Mutating && s.Command != "" && gathered[stepCheck(s.Command)] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// pinFix marks the trailing fix of a Pro message, a command that
// changes the cluster and the sentence before it, so a provider's
// length limit cuts the proof above it and never the fix.
func pinFix(doc []notification.Block, steps []notification.Step) {
	n := len(doc)
	if n < 2 || doc[n-1].Kind != notification.CodeBlock {
		return
	}
	command := doc[n-1].Text()
	for _, s := range steps {
		if s.Mutating && strings.TrimSpace(s.Command) == command {
			doc[n-1].Pinned, doc[n-2].Pinned = true, true
			return
		}
	}
}
