package scenarios

import (
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// Non-events are healthy changes a busy cluster makes all day. None of
// them may cause a notification. Their objects are named so a
// notification can be traced back to them: namespaced objects live in
// the background namespace or one starting with calmPrefix, and
// cluster-scoped ones (nodes, zones) start with calmPrefix or
// backgroundPrefix.
const (
	calmPrefix          = "calm"
	backgroundPrefix    = "bg-"
	backgroundNamespace = "staging"
)

// nonEventCase is one healthy change of the staging day.
type nonEventCase struct {
	name  string
	build func(c *cluster)
}

// nonEventCases are the explicit non-events. The background churn
// (rollouts and scale events of the staging workloads) is a non-event
// too, and is attributed the same way.
func nonEventCases() []nonEventCase {
	return []nonEventCase{
		{"successful rollout", calmRollout},
		{"scale up and down", calmScale},
		{"node drain within its budget", calmDrain},
		{"pod churn from Jobs completing", calmJobs},
		{"healthy CronJob", calmCronJob},
	}
}

// nonEventLogs records every non-event case twice, spread over the day.
func nonEventLogs() []replay.Log {
	cases := nonEventCases()
	runs := 2 * len(cases)
	spacing := stagingDay() / time.Duration(runs+1)
	logs := make([]replay.Log, 0, runs)
	for i := range runs {
		nc := cases[i%len(cases)]
		start := stagingStart.Add(spacing*time.Duration(i+1) +
			jitter(nc.name, spacing/4))
		c := newCluster(start, fmt.Sprintf("-c%d", i))
		nc.build(c)
		logs = append(logs, c.log())
	}
	return logs
}

// nonEventNotifications counts the messages whose incident involves a
// non-event entity: its root, a finding it explains or its impact. loud
// counts those of them that interrupt (notify or page); the rest are
// low-priority (digest) messages, which are still notifications.
func nonEventNotifications(
	decisions []incident.Decision,
) (all, loud int) {
	for _, d := range decisions {
		if !involvesNonEvent(d.Incident) {
			continue
		}
		all++
		if d.Incident.Tier >= incident.Notify {
			loud++
		}
	}
	return all, loud
}

func involvesNonEvent(p incident.Incident) bool {
	if isNonEvent(p.Root) {
		return true
	}
	for key := range p.Members {
		if isNonEvent(key.Entity) {
			return true
		}
	}
	for _, id := range p.Impact {
		if isNonEvent(id) {
			return true
		}
	}
	return false
}

func isNonEvent(id inventory.EntityID) bool {
	if id.Namespace != "" {
		return id.Namespace == backgroundNamespace ||
			strings.HasPrefix(id.Namespace, calmPrefix)
	}
	return strings.HasPrefix(id.Name, calmPrefix) ||
		strings.HasPrefix(id.Name, backgroundPrefix)
}

// calmNodes lists three ready nodes and returns their names.
func calmNodes(c *cluster) []string {
	names := []string{"calm-n1", "calm-n2", "calm-n3"}
	for _, name := range names {
		c.list(c.node(name, "calm-zone"))
	}
	return names
}

// calmFleet lists a healthy Deployment of replicas spread over nodes.
func calmFleet(c *cluster, name string, replicas int,
	nodes []string) *workload {
	w := c.deployment(calmPrefix, name,
		"registry.example.com/"+name+":1", int32(replicas))
	c.list(w.objects())
	for r := range replicas {
		c.list(w.pod(r, nodes[r%len(nodes)]))
	}
	return w
}

// calmRollout: a Deployment rolls out a new image; new pods take 25
// seconds to become ready, then the old ones go.
func calmRollout(c *cluster) {
	nodes := calmNodes(c)
	w := calmFleet(c, "api", 3, nodes)
	c.after(time.Minute)
	successfulRollout(c, w, nodes)
	c.after(10 * time.Minute)
}

// calmScale: a Deployment scales from three to five replicas and,
// twenty minutes later, back.
func calmScale(c *cluster) {
	nodes := calmNodes(c)
	w := calmFleet(c, "worker", 3, nodes)
	c.after(time.Minute)
	scaleOut(c, w, nodes)
	c.after(scaleHold)
	scaleBack(c, w)
	c.after(10 * time.Minute)
}

// calmDrain: calm-n3 is cordoned and drained for an upgrade. Each pod
// on it is evicted through the eviction API (the budget allows one
// disruption), its replacement is ready on another node within half a
// minute, and the empty node is deleted.
func calmDrain(c *cluster) {
	nodes := calmNodes(c)
	fleet := []*workload{calmFleet(c, "web", 3, nodes),
		calmFleet(c, "cache", 3, nodes)}
	c.after(time.Minute)
	cordoned := last(c, c.node("calm-n3", "calm-zone"))
	cordoned.Spec.Unschedulable = true
	cordoned.Spec.Taints = []corev1.Taint{{
		Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
	}}
	c.update(cordoned)
	for _, w := range fleet {
		pod := w.pod(2, "calm-n3")
		now := metav1.NewTime(c.now)
		pod.DeletionTimestamp = &now
		nodeDisruption(c, pod, "EvictionByEvictionAPI",
			"Eviction API: evicting")
		c.update(pod)
		c.remove(pod)
		w.setReady(2)
		c.update(w.objects())
		c.create(w.pod(3, "calm-n1", startedNow, notReady))
		c.after(20 * time.Second)
		c.update(w.pod(3, "calm-n1", startedNow))
		w.setReady(3)
		c.update(w.objects())
	}
	c.after(2 * time.Minute)
	c.remove(last(c, cordoned))
	c.after(10 * time.Minute)
}

// calmJobs: five one-off Jobs run a pod each to completion; finished
// Jobs and their pods are deleted five minutes later (TTL after
// finished).
func calmJobs(c *cluster) {
	c.list(c.node("calm-n1", "calm-zone"))
	for i := range 5 {
		job := calmJob(c, fmt.Sprintf("migrate-%d", i), nil)
		calmRunJob(c, job, "calm-n1")
		c.after(2 * time.Minute)
		c.remove(last(c, job))
	}
	c.after(10 * time.Minute)
}

// calmCronJob: a CronJob runs on its schedule, every 30 minutes, for
// three hours; each run's Job finishes in a minute, and the history
// limit removes the oldest Job. Its listed status shows the previous
// run on time and successful. It is deleted before its next run.
func calmCronJob(c *cluster) {
	c.list(c.node("calm-n1", "calm-zone"))
	cron := scheduleCronJob(c, calmPrefix, "rollup", "*/30 * * * *")
	previous := metav1.NewTime(c.now.Truncate(30 * time.Minute))
	cron.Status.LastScheduleTime = &previous
	cron.Status.LastSuccessfulTime = &previous
	c.list(cron)
	var jobs []*batchv1.Job
	for run := range 6 {
		// The controller starts each run a few seconds after it is due.
		c.now = c.now.Truncate(30 * time.Minute).Add(30*time.Minute +
			3*time.Second)
		due := metav1.NewTime(c.now.Truncate(30 * time.Minute))
		job := calmJob(c, fmt.Sprintf("rollup-%d", run), cron)
		scheduled := last(c, cron)
		scheduled.Status.LastScheduleTime = &due
		c.update(scheduled)
		calmRunJob(c, job, "calm-n1")
		done := metav1.NewTime(c.now)
		finished := last(c, cron)
		finished.Status.LastSuccessfulTime = &done
		c.update(finished)
		jobs = append(jobs, job)
		if len(jobs) > 3 {
			c.remove(last(c, jobs[0]))
			jobs = jobs[1:]
		}
	}
	// The case ends before the next run is due: the CronJob and its
	// history are deleted, as a finished staging test cleans up.
	c.after(10 * time.Minute)
	for _, job := range jobs {
		c.remove(last(c, job))
	}
	c.remove(last(c, cron))
}

// calmJob builds a Job in the calm namespace, owned by cron when set.
func calmJob(c *cluster, name string, cron *batchv1.CronJob) *batchv1.Job {
	meta := c.meta(calmPrefix, name)
	meta.CreationTimestamp = metav1.NewTime(c.now)
	meta.Labels = map[string]string{"job-name": meta.Name}
	if cron != nil {
		yes := true
		meta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "batch/v1", Kind: "CronJob", Name: cron.Name,
			UID: cron.UID, Controller: &yes,
		}}
	}
	return &batchv1.Job{
		ObjectMeta: meta,
		Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: meta.Labels},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{{
					Name: "task", Image: "registry.example.com/task:2",
				}},
			},
		}},
	}
}

// calmRunJob runs job's one pod to a clean exit and completes the Job.
func calmRunJob(c *cluster, job *batchv1.Job, node string) {
	started := metav1.NewTime(c.now)
	job.Status = batchv1.JobStatus{Active: 1, StartTime: &started}
	c.create(job)
	pod := calmJobPod(c, job, node)
	c.create(pod)
	c.after(time.Minute)
	finished := metav1.NewTime(c.now)
	pod = pod.DeepCopy()
	pod.Status.Phase = corev1.PodSucceeded
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 0, Reason: "Completed", StartedAt: started,
			FinishedAt: finished,
		},
	}
	pod.Status.ContainerStatuses[0].Ready = false
	c.update(pod)
	done := job.DeepCopy()
	done.Status = batchv1.JobStatus{
		Succeeded: 1, StartTime: &started, CompletionTime: &finished,
		Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobSuccessCriteriaMet,
				Status: corev1.ConditionTrue, LastTransitionTime: finished},
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
				LastTransitionTime: finished},
		},
	}
	c.update(done)
	c.after(time.Minute)
	c.remove(pod)
}

// calmJobPod is the running pod of a Job.
func calmJobPod(c *cluster, job *batchv1.Job, node string) *corev1.Pod {
	yes := true
	name := job.Name + "-x"
	started := metav1.NewTime(c.now)
	container := job.Spec.Template.Spec.Containers[0]
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: job.Namespace, UID: types.UID(name),
			Labels: job.Labels, CreationTimestamp: started,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "Job", Name: job.Name,
				UID: job.UID, Controller: &yes,
			}},
		},
		Spec: corev1.PodSpec{
			NodeName: c.n(node), RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{container},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning, StartTime: &started,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: container.Name, Image: container.Image, Ready: true,
				Started: boolPtr(true),
				State: corev1.ContainerState{Running: &corev1.
					ContainerStateRunning{StartedAt: started}},
			}},
		},
	}
}
