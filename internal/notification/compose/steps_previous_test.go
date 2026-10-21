package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A container that never restarted has no previous run: --previous
// would fail with "previous terminated container not found".
func TestStepsReadCurrentLogsForContainersThatNeverRestarted(t *testing.T) {
	id := inventory.CoreID(kube.KindContainer, "shop", "web-7d9f-a/app")
	for _, finding := range []detection.Finding{
		{Entity: id, Reason: reasons.ReadinessProbeFailed},
		{Entity: id, Reason: reasons.ContainerCPUThrottled},
		{Entity: id, Reason: reasons.Error},
	} {
		got := stepsFor(id, finding)

		assert.NotContains(t, got, "--previous", finding.Reason)
		assert.Contains(t, got, "kubectl describe pod web-7d9f-a -n shop",
			finding.Reason)
		assert.Contains(t, got,
			"kubectl logs web-7d9f-a -c app -n shop", finding.Reason)
	}
}

func TestStepsReadPreviousLogsWhenTheContainerRestarted(t *testing.T) {
	id := inventory.CoreID(kube.KindContainer, "shop", "web-7d9f-a/app")
	for _, finding := range []detection.Finding{
		{Entity: id, Reason: reasons.CrashLoopBackOff},
		{Entity: id, Reason: reasons.LivenessKilled},
		{Entity: id, Reason: reasons.Error, Evidence: []detection.Evidence{
			{Label: "restarts", Value: "3"}}},
		{Entity: id, Reason: reasons.OOMKilled, Evidence: []detection.Evidence{
			{Label: "last termination", Value: "OOMKilled"}}},
	} {
		got := stepsFor(id, finding)

		assert.Contains(t, got, "--previous", finding.Reason)
	}
}

// The cluster's own components have no object to describe; the page
// still opens with the first thing to look at, by the labels every
// Kubernetes install gives them.
func TestStepsOpenControlPlanePagesWithAFirstStep(t *testing.T) {
	for _, c := range []struct {
		name string
		root inventory.EntityID
		want []string
	}{
		{"cluster DNS", kube.ClusterDNS, []string{
			"kubectl -n kube-system get pods -l k8s-app=kube-dns",
			"kubectl -n kube-system logs -l k8s-app=kube-dns --tail=50"}},
		{"scheduler", kube.Scheduler, []string{
			"kubectl -n kube-system get pods -l component=kube-scheduler",
			"kubectl -n kube-system logs -l component=kube-scheduler " +
				"--tail=50"}},
		{"controller manager", kube.ControllerManager, []string{
			"kubectl -n kube-system get pods " +
				"-l component=kube-controller-manager",
			"kubectl -n kube-system logs " +
				"-l component=kube-controller-manager --tail=50"}},
		{"API server", kube.APIServer, []string{
			"kubectl get --raw '/readyz?verbose'"}},
		{"etcd", kube.Etcd, []string{
			"kubectl get --raw '/readyz?verbose'"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := incident.Incident{Root: c.root}

			steps := nextSteps(p, []detection.Finding{{Entity: c.root,
				Reason: "Failing"}})

			var commands []string
			for _, step := range steps {
				commands = append(commands, step.Command)
				assert.False(t, step.Mutating)
			}
			assert.Equal(t, c.want, commands)
			assert.False(t, strings.Contains(strings.Join(commands, " "),
				"describe"))
		})
	}
}
