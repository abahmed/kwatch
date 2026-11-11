package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// ownedSteps are the commands for one finding of a container whose pod
// belongs to root.
func ownedSteps(
	root inventory.EntityID, pod string, f detection.Finding,
) []string {
	f.Entity = inventory.CoreID(kube.KindContainer, root.Namespace,
		pod+"/app")
	p := incident.Incident{Root: root}
	var out []string
	for _, step := range nextSteps(p, []detection.Finding{f}) {
		out = append(out, step.Command)
	}
	return out
}

const pickWeb = "kubectl get pods -n shop -o wide | " +
	"grep -E '^(NAME|POD|web-)'"

// A controller's pod is replaced under a new name by a restart, a
// rollout or an eviction: a step that names it fails with NotFound
// minutes later. The steps read it through the controller instead.
func TestStepsReadAControllersPodThroughTheController(t *testing.T) {
	crash := detection.Finding{Reason: reasons.CrashLoopBackOff}
	for _, c := range []struct {
		name string
		root inventory.EntityID
		want string
	}{
		{"deployment", inventory.CoreID(kube.KindDeployment, "shop", "web"),
			"kubectl logs deploy/web -c app -n shop --previous"},
		{"daemonset", inventory.CoreID(kube.KindDaemonSet, "shop", "web"),
			"kubectl logs ds/web -c app -n shop --previous"},
		{"replicaset", inventory.CoreID(kube.KindReplicaSet, "shop", "web"),
			"kubectl logs rs/web -c app -n shop --previous"},
		{"job", inventory.CoreID(kube.KindJob, "shop", "web"),
			"kubectl logs job/web -c app -n shop --previous"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := ownedSteps(c.root, "web-7d9f-a", crash)

			assert.Equal(t, []string{c.want, pickWeb}, got)
		})
	}
}

// kubectl reads one pod of a controller, the most available one, which
// may have no previous run: the next step says how to read another.
func TestPickPodStepNamesTheControllerRef(t *testing.T) {
	ref := podRefOf([]inventory.EntityID{
		inventory.CoreID(kube.KindDeployment, "shop", "web")},
		inventory.CoreID(kube.KindContainer, "shop", "web-7d9f-a/app"))

	steps := ref.pickPodStep()

	assert.Equal(t, []notification.Step{{
		Text: "kubectl reads only one of web's pods; to read another, " +
			"find it in this list and use its name in place of deploy/web",
		Command: pickWeb,
	}}, steps)
}

// A StatefulSet's pod keeps its name when replaced, a bare pod has no
// controller, and a CronJob cannot be read by kubectl logs: those keep
// the pod's own name.
func TestStepsKeepThePodNameWhenItIsStable(t *testing.T) {
	crash := detection.Finding{Reason: reasons.CrashLoopBackOff}
	for _, c := range []struct {
		name string
		root inventory.EntityID
		pod  string
	}{
		{"statefulset", inventory.CoreID(kube.KindStatefulSet, "shop",
			"db"), "db-0"},
		{"bare pod", inventory.CoreID(kube.KindPod, "shop", "web-7d9f-a"),
			"web-7d9f-a"},
		{"cronjob", inventory.CoreID(kube.KindCronJob, "shop", "web"),
			"web-29170-a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := ownedSteps(c.root, c.pod, crash)

			assert.Equal(t, []string{"kubectl logs " + c.pod +
				" -c app -n shop --previous"}, got)
		})
	}
}

// The longest controller name wins, as the pod is named after it.
func TestPodRefPicksTheLongestMatchingController(t *testing.T) {
	ref := podRefOf([]inventory.EntityID{
		inventory.CoreID(kube.KindDeployment, "shop", "web"),
		inventory.CoreID(kube.KindDeployment, "shop", "web-api"),
	}, inventory.CoreID(kube.KindContainer, "shop", "web-api-7d9f-a/app"))

	assert.Equal(t, "deploy/web-api -c app", ref.logs())
	assert.Equal(t, "pods web-api-", ref.describe())

	other := podRefOf([]inventory.EntityID{
		inventory.CoreID(kube.KindDeployment, "pay", "web"),
	}, inventory.CoreID(kube.KindContainer, "shop", "web-7d9f-a/app"))
	assert.Equal(t, "web-7d9f-a -c app", other.logs())
}

func TestStepsDescribeEveryPodOfTheController(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	for _, c := range []struct {
		name    string
		finding detection.Finding
		want    []string
	}{
		{"never started", detection.Finding{
			Reason: reasons.CreateConfigError},
			[]string{"kubectl describe pods web- -n shop"}},
		{"never restarted", detection.Finding{
			Reason: reasons.ReadinessProbeFailed},
			[]string{"kubectl describe pods web- -n shop",
				"kubectl logs deploy/web -c app -n shop", pickWeb}},
		{"init container", detection.Finding{
			Reason: reasons.InitContainerWaiting},
			[]string{"kubectl logs deploy/web -c app -n shop", pickWeb}},
		{"evicted", detection.Finding{Reason: reasons.Evicted},
			[]string{"kubectl describe pods web- -n shop"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, ownedSteps(web, "web-7d9f-a", c.finding))
		})
	}
}

func TestStepsCompareMemoryOfEveryPodOfTheController(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")

	got := ownedSteps(web, "web-7d9f-a",
		detection.Finding{Reason: reasons.OOMKilled,
			Evidence: []detection.Evidence{
				{Label: "last termination", Value: "OOMKilled"}}})

	assert.Equal(t, []string{
		"kubectl logs deploy/web -c app -n shop --previous", pickWeb,
		"kubectl top pod -n shop --containers | " +
			"grep -E '^(NAME|POD|web-)'",
	}, got)
}

// A controller found in the impact, not the root, is used too.
func TestStepsFindTheControllerInTheImpact(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	ct := inventory.CoreID(kube.KindContainer, "shop", "web-7d9f-a/app")
	p := incident.Incident{
		Root:   inventory.CoreID(kube.KindConfigMap, "shop", "conf"),
		Impact: []inventory.EntityID{web},
	}

	steps := symptomSteps(knownOwners(p), []detection.Finding{{Entity: ct,
		Reason: reasons.CrashLoopBackOff}})

	assert.Equal(t, "kubectl logs deploy/web -c app -n shop --previous",
		steps[0].Command)
}

func TestNewPodCommandsMapToThePodStateCheck(t *testing.T) {
	assert.Equal(t, incident.CheckPodState, stepCheck(pickWeb))
	assert.Equal(t, incident.CheckPodState,
		stepCheck("kubectl describe pods web- -n shop"))
	assert.Equal(t, incident.CheckUsage,
		stepCheck("kubectl top pod -n shop --containers"))
}

// A pending pod is read through its controller too: a new pod with a
// new name may take its place.
func TestStepsDescribeAPendingPodWithTheControllersPods(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-7d9f-a")

	steps := nextSteps(incident.Incident{Root: web},
		[]detection.Finding{{Entity: pod, Reason: "Unschedulable"}})

	assert.Equal(t, []notification.Step{{
		Text:    "See the pods' state and events",
		Command: "kubectl describe pods web- -n shop",
	}}, steps)
}
