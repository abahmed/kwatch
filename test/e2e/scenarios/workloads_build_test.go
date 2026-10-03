//go:build e2e

package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
)

// deploymentOption changes a Deployment built by deployment.
type deploymentOption func(*appsv1.Deployment)

// withReplicas sets how many Pods the Deployment runs.
func withReplicas(replicas int32) deploymentOption {
	return func(d *appsv1.Deployment) { d.Spec.Replicas = ptr(replicas) }
}

// onNode pins the Pods to one node.
func onNode(node string) deploymentOption {
	return func(d *appsv1.Deployment) {
		d.Spec.Template.Spec.NodeName = node
	}
}

// withConfigMapEnv makes the Pods read all of their environment from a
// ConfigMap, so they cannot start until it exists.
func withConfigMapEnv(configMap string) deploymentOption {
	return func(d *appsv1.Deployment) {
		container := &d.Spec.Template.Spec.Containers[0]
		container.EnvFrom = []corev1.EnvFromSource{
			configMapEnvFrom(configMap),
		}
	}
}

// withCPURequest sets the CPU request of the workload container.
func withCPURequest(quantity string) deploymentOption {
	return func(d *appsv1.Deployment) {
		container := &d.Spec.Template.Spec.Containers[0]
		container.Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(quantity),
		}
	}
}

// withRemoteImage replaces the workload image with one the node must pull
// from a registry.
func withRemoteImage(image string) deploymentOption {
	return func(d *appsv1.Deployment) {
		container := &d.Spec.Template.Spec.Containers[0]
		container.Image = image
		container.ImagePullPolicy = corev1.PullIfNotPresent
	}
}

// appLabels is the label that ties a workload to its selector.
func appLabels(app string) map[string]string {
	return map[string]string{"app": app}
}

// podTemplate wraps a Pod spec in a template labelled with the app name.
func podTemplate(app string, spec corev1.PodSpec) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: appLabels(app)},
		Spec:       spec,
	}
}

// selectorFor selects the Pods labelled with the app name.
func selectorFor(app string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: appLabels(app)}
}

// deployment is a one-replica Deployment whose container runs the workload
// in the given mode, changed by the options.
func deployment(
	name, mode string, options ...deploymentOption,
) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Selector: selectorFor(name),
			Template: podTemplate(name, workloadPod(name, mode).Spec),
		},
	}
	for _, option := range options {
		option(d)
	}
	return d
}

// stuckStatefulSet never starts: its image is not on the node.
func stuckStatefulSet(name string) *appsv1.StatefulSet {
	spec := podWithMissingImage(name).Spec
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name, Replicas: ptr(int32(1)),
			Selector: selectorFor(name),
			Template: podTemplate(name, spec),
		},
	}
}

// stuckDaemonSet never starts: its image is not on the node.
func stuckDaemonSet(name string) *appsv1.DaemonSet {
	spec := podWithMissingImage(name).Spec
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Selector: selectorFor(name),
			Template: podTemplate(name, spec),
		},
	}
}

// healthyReplicaSet is healthy, so only a quota can stop it.
func healthyReplicaSet(name string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr(int32(1)), Selector: selectorFor(name),
			Template: podTemplate(name, workloadPod(name, "healthy").Spec),
		},
	}
}

// failingJob fails at once and is never retried.
func failingJob(name string) *batchv1.Job {
	spec := crashingPod(name).Spec
	spec.RestartPolicy = corev1.RestartPolicyNever
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(0)),
			Template:     corev1.PodTemplateSpec{Spec: spec},
		},
	}
}

// suspendedCronJob would run every five minutes if it were not suspended.
func suspendedCronJob(name string) *batchv1.CronJob {
	spec := workloadPod(name, "healthy").Spec
	spec.RestartPolicy = corev1.RestartPolicyNever
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *", Suspend: ptr(true),
			JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{Spec: spec},
			}},
		},
	}
}

// podDisruptionBudget requires one Pod of the app to stay available.
func podDisruptionBudget(name, app string) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable, Selector: selectorFor(app),
		},
	}
}

// zeroPodQuota forbids creating any Pod in its namespace.
func zeroPodQuota(name string) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			corev1.ResourcePods: resource.MustParse("0"),
		}},
	}
}

// cpuAutoscaler scales the named Deployment on CPU, so the autoscaler asks
// the metrics API for numbers.
func cpuAutoscaler(target string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: target},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: target,
			},
			MinReplicas: ptr(int32(1)), MaxReplicas: 2,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: ptr(int32(50)),
					},
				},
			}},
		},
	}
}

// CreateDeployment creates a one-replica Deployment whose container runs
// the workload in the given mode ("crash", "healthy", "sleep", ...).
func (s *Scenario) CreateDeployment(
	name, mode string, options ...deploymentOption,
) {
	s.T.Helper()
	_, err := s.Env.Client.AppsV1().Deployments(s.Namespace).Create(
		s.Ctx, deployment(name, mode, options...), metav1.CreateOptions{})
	s.Must(err)
}

// CreateStatefulSet creates the StatefulSet in the scenario namespace.
func (s *Scenario) CreateStatefulSet(set *appsv1.StatefulSet) {
	s.T.Helper()
	_, err := s.Env.Client.AppsV1().StatefulSets(s.Namespace).Create(
		s.Ctx, set, metav1.CreateOptions{})
	s.Must(err)
}

// CreateDaemonSet creates the DaemonSet in the scenario namespace.
func (s *Scenario) CreateDaemonSet(set *appsv1.DaemonSet) {
	s.T.Helper()
	_, err := s.Env.Client.AppsV1().DaemonSets(s.Namespace).Create(
		s.Ctx, set, metav1.CreateOptions{})
	s.Must(err)
}

// CreateReplicaSet creates the ReplicaSet in the scenario namespace.
func (s *Scenario) CreateReplicaSet(set *appsv1.ReplicaSet) {
	s.T.Helper()
	_, err := s.Env.Client.AppsV1().ReplicaSets(s.Namespace).Create(
		s.Ctx, set, metav1.CreateOptions{})
	s.Must(err)
}

// CreateJob creates the Job in the scenario namespace.
func (s *Scenario) CreateJob(job *batchv1.Job) {
	s.T.Helper()
	_, err := s.Env.Client.BatchV1().Jobs(s.Namespace).Create(
		s.Ctx, job, metav1.CreateOptions{})
	s.Must(err)
}

// CreateCronJob creates the CronJob in the scenario namespace.
func (s *Scenario) CreateCronJob(job *batchv1.CronJob) {
	s.T.Helper()
	_, err := s.Env.Client.BatchV1().CronJobs(s.Namespace).Create(
		s.Ctx, job, metav1.CreateOptions{})
	s.Must(err)
}

// CreatePodDisruptionBudget creates the budget in the scenario namespace.
func (s *Scenario) CreatePodDisruptionBudget(
	budget *policyv1.PodDisruptionBudget,
) {
	s.T.Helper()
	budgets := s.Env.Client.PolicyV1().PodDisruptionBudgets(s.Namespace)
	_, err := budgets.Create(s.Ctx, budget, metav1.CreateOptions{})
	s.Must(err)
}

// CreateResourceQuota creates the quota in the scenario namespace.
func (s *Scenario) CreateResourceQuota(quota *corev1.ResourceQuota) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().ResourceQuotas(s.Namespace).Create(
		s.Ctx, quota, metav1.CreateOptions{})
	s.Must(err)
}

// CreateAutoscaler creates the HorizontalPodAutoscaler in the namespace.
func (s *Scenario) CreateAutoscaler(
	autoscaler *autoscalingv2.HorizontalPodAutoscaler,
) {
	s.T.Helper()
	autoscalers := s.Env.Client.AutoscalingV2().
		HorizontalPodAutoscalers(s.Namespace)
	_, err := autoscalers.Create(s.Ctx, autoscaler, metav1.CreateOptions{})
	s.Must(err)
}

// WaitForRollout waits until every Pod of the Deployment is available.
func (s *Scenario) WaitForRollout(deployment string) {
	s.T.Helper()
	s.Must(s.Env.WaitForDeployment(s.Ctx, s.Namespace, deployment))
}

// BreakRollout points the Deployment at a missing image with a short
// progress deadline. It re-reads the Deployment on every attempt, because
// the Deployment controller updates it concurrently.
func (s *Scenario) BreakRollout(name string) {
	s.T.Helper()
	deployments := s.Env.Client.AppsV1().Deployments(s.Namespace)
	s.Must(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := deployments.Get(s.Ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		current.Spec.Template.Spec.Containers[0].Image = missingImage
		current.Spec.ProgressDeadlineSeconds = ptr(int32(5))
		_, err = deployments.Update(s.Ctx, current, metav1.UpdateOptions{})
		return err
	}))
}

// RestartPods replaces the Pods of a Deployment by changing an annotation
// of its Pod template.
func (s *Scenario) RestartPods(deployment string) {
	s.T.Helper()
	patch := `{"spec":{"template":{"metadata":{"annotations":` +
		`{"kwatch-e2e/restarted":"` +
		time.Now().Format(time.RFC3339Nano) + `"}}}}}`
	_, err := s.Env.Client.AppsV1().Deployments(s.Namespace).Patch(s.Ctx,
		deployment, types.StrategicMergePatchType, []byte(patch),
		metav1.PatchOptions{})
	s.Must(err)
}
