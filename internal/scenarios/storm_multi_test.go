package scenarios

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// The multi-cause storm is 1,000 failing pods from ten independent
// causes across namespaces, all within one minute: four nodes stop
// reporting (one per zone), three private registries reject pulls, and
// three namespaces lose a Secret their new workloads need. Each cause
// breaks 50 two-replica workloads. People must hear about each cause
// once, correctly, without a message per workload.
const (
	multiStormLostNodes  = 4
	multiStormRegistries = 3
	multiStormSecrets    = 3
	// multiStormFleet is how many workloads one cause breaks.
	multiStormFleet = 50
)

// multiStormCauses is the number of independent causes.
const multiStormCauses = multiStormLostNodes + multiStormRegistries +
	multiStormSecrets

// multiStormSlack is how many messages beyond one per cause the storm
// may send in two minutes, for example a cause revised once.
const multiStormSlack = 3

// multiStormResult is how loud the multi-cause storm was and how many of
// its causes people were told correctly.
type multiStormResult struct {
	stormResult
	causes int
	// found counts the causes reported as the root of their own incident
	// with a stated cause.
	found int
	// missed names the expected roots no incident was rooted at.
	missed []string
}

func runMultiCauseStorm(t testing.TB) multiStormResult {
	t.Helper()
	log, roots := multiCauseStorm()
	result := replayLog(t, log, replay.Options{})
	out := multiStormResult{
		stormResult: stormResult{
			name:     "multi-cause",
			pods:     multiStormCauses * multiStormFleet * stormReplicas,
			messages: len(result.Messages),
			peak: scorecard.PeakInWindow(result.Times,
				scorecard.GoalStormWindow),
		},
		causes: len(roots),
	}
	stated := map[string]bool{}
	for _, r := range reportedIncidents(result) {
		if r.Cause != nil {
			stated[r.Root] = true
		}
	}
	for _, root := range roots {
		if stated[root] {
			out.found++
		} else {
			out.missed = append(out.missed, root)
		}
	}
	return out
}

// multiStormGates are the gates of the multi-cause storm: at most one
// message per cause plus slack in any two minutes, and every cause
// reported correctly.
func multiStormGates(r multiStormResult) []scorecard.Gate {
	found := scorecard.AtLeast("Multi-cause storm causes reported",
		float64(r.found), float64(r.causes), "")
	if len(r.missed) > 0 {
		found.Value += fmt.Sprintf(" (missed %v)", r.missed)
	}
	return []scorecard.Gate{
		scorecard.AtMost("Storm messages in 2 minutes (multi-cause)",
			float64(r.peak), float64(r.causes+multiStormSlack), ""),
		found,
	}
}

// multiCauseStorm builds the storm log and returns the roots that must
// be reported.
func multiCauseStorm() (replay.Log, []string) {
	c := newCluster(stormStart, "")
	var roots []string
	var fail []func()
	for k := range multiStormLostNodes {
		root, breaks := multiStormLostNode(c, k)
		roots, fail = append(roots, root), append(fail, breaks)
	}
	for k := range multiStormRegistries {
		root, breaks := multiStormRegistry(c, k)
		roots, fail = append(roots, root), append(fail, breaks)
	}
	for k := range multiStormSecrets {
		root, breaks := multiStormSecret(c, k)
		roots, fail = append(roots, root), append(fail, breaks)
	}
	c.after(time.Minute)
	for _, breaks := range fail {
		breaks()
		c.after(5 * time.Second)
	}
	return c.log(), roots
}

// multiStormLostNode lists a node in its own zone, a healthy spare next
// to it and 50 workloads on it. Breaking it stops the node reporting;
// every pod on it turns not ready.
func multiStormLostNode(c *cluster, k int) (string, func()) {
	zone := fmt.Sprintf("zone-%c", 'a'+k)
	name := fmt.Sprintf("lost-%d", k)
	node := c.node(name, zone)
	c.list(node, c.node(fmt.Sprintf("spare-%d", k), zone))
	fleet := multiStormWorkloads(c, fmt.Sprintf("nodes-%d", k),
		"registry.example.com/fleet", func(int) string { return name })
	return "node//" + name, func() {
		lost := last(c, node)
		setNodeCondition(lost, corev1.NodeReady, corev1.ConditionUnknown,
			"NodeStatusUnknown", "Kubelet stopped posting node status.",
			c.now)
		c.update(lost)
		c.after(40 * time.Second)
		for _, w := range fleet {
			for i := range stormReplicas {
				c.update(w.pod(i, name, notReady))
			}
			w.setReady(0)
			c.update(w.objects())
		}
	}
}

// multiStormRegistry lists 50 workloads pulling from one private
// registry. Breaking it recreates their pods on fresh nodes, where every
// pull fails with 401 Unauthorized.
func multiStormRegistry(c *cluster, k int) (string, func()) {
	host := fmt.Sprintf("registry-%d.corp.example", k)
	pool := func(prefix string) func(int) string {
		return func(i int) string {
			return fmt.Sprintf("%s-%d", prefix, i%5)
		}
	}
	old := pool(fmt.Sprintf("pool-%d", k))
	for i := range 5 {
		c.list(c.node(old(i), "zone-e"))
	}
	fleet := multiStormWorkloads(c, fmt.Sprintf("images-%d", k),
		host+"/base", old)
	return "registry//" + host, func() {
		fresh := pool(fmt.Sprintf("fresh-%d", k))
		for i := range 5 {
			c.create(c.node(fresh(i), "zone-e"))
		}
		for i, w := range fleet {
			image := w.deployment.Spec.Template.Spec.Containers[0].Image
			message := "Back-off pulling image \"" + image + "\": " +
				"ErrImagePull: failed to pull and unpack image \"" +
				image + "\": failed to resolve reference: pulling from " +
				"host " + host + " failed with status code [manifests " +
				"v1]: 401 Unauthorized"
			for r := range stormReplicas {
				c.remove(last(c, w.pod(r, "")))
				c.create(w.pod(r+stormReplicas,
					fresh(i*stormReplicas+r), startedNow,
					waiting("ImagePullBackOff", message)))
			}
			w.setReady(0)
			c.update(w.objects())
		}
	}
}

// multiStormSecret creates, when broken, 50 new workloads in one
// namespace that read a Secret that does not exist; their pods fail with
// CreateContainerConfigError.
func multiStormSecret(c *cluster, k int) (string, func()) {
	namespace := fmt.Sprintf("secrets-%d", k)
	secret := fmt.Sprintf("creds-%d", k)
	node := fmt.Sprintf("apps-%d", k)
	c.list(c.node(node, "zone-f"))
	return "secret/" + namespace + "/" + secret, func() {
		message := "secret \"" + secret + "\" not found"
		for i := range multiStormFleet {
			w := c.deployment(namespace, fmt.Sprintf("app-%d", i),
				fmt.Sprintf("registry.example.com/app-%d:1.0", i),
				stormReplicas)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret,
						},
					},
				}}
			})
			w.setReady(0)
			c.create(w.objects())
			for r := range stormReplicas {
				c.create(w.pod(r, node, startedNow,
					waiting("CreateContainerConfigError", message)))
			}
			configWarn(c, w, 0, stormReplicas, "Failed", "Error: "+message)
		}
	}
}

// multiStormWorkloads lists 50 healthy two-replica Deployments in
// namespace, placing replica r of workload i on place(i*2+r).
func multiStormWorkloads(
	c *cluster, namespace, repository string, place func(int) string,
) []*workload {
	fleet := make([]*workload, 0, multiStormFleet)
	for i := range multiStormFleet {
		w := c.deployment(namespace, fmt.Sprintf("svc-%d", i),
			fmt.Sprintf("%s/svc-%d:v1", repository, i), stormReplicas)
		c.list(w.objects())
		for r := range stormReplicas {
			c.list(w.pod(r, place(i*stormReplicas+r)))
		}
		fleet = append(fleet, w)
	}
	return fleet
}

func TestMultiCauseStormIsThousandPods(t *testing.T) {
	if pods := multiStormCauses * multiStormFleet * stormReplicas; pods !=
		1000 {
		t.Fatalf("storm has %d pods, want 1000", pods)
	}
}
