//go:build e2e

package scenarios

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func edgeSleepContainer() corev1.Container {
	return corev1.Container{
		Name: "workload", Image: workloadImage(),
		Command:         []string{"/kwatch-e2e-workload", "sleep"},
		ImagePullPolicy: corev1.PullNever,
	}
}

func edgeCreatePod(
	ctx context.Context, e *harness.Environment, namespace string,
	pod *corev1.Pod,
) error {
	_, err := e.Client.CoreV1().Pods(namespace).Create(
		ctx, pod, metav1.CreateOptions{})
	return err
}

func edgeFailingHookPod(name string) *corev1.Pod {
	c := edgeSleepContainer()
	c.Lifecycle = &corev1.Lifecycle{PostStart: &corev1.LifecycleHandler{
		Exec: &corev1.ExecAction{Command: []string{
			"/kwatch-e2e-workload", "startup-error",
		}},
	}}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{c}},
	}
}

func edgeCreateSuspendedCronJob(
	ctx context.Context, e *harness.Environment, namespace, name string,
) error {
	suspended := true
	job := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *", Suspend: &suspended,
			JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name: "workload", Image: workloadImage(),
						Command: []string{
							"/kwatch-e2e-workload", "healthy",
						},
						ImagePullPolicy: corev1.PullNever,
					}},
				}},
			}},
		},
	}
	_, err := e.Client.BatchV1().CronJobs(namespace).Create(
		ctx, job, metav1.CreateOptions{})
	return err
}

func edgePodWithEnvFrom(name string, from corev1.EnvFromSource) *corev1.Pod {
	c := edgeSleepContainer()
	c.EnvFrom = []corev1.EnvFromSource{from}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{c}},
	}
}

func edgeSecretEnvFrom(name string) corev1.EnvFromSource {
	return corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
	}}
}

func edgeConfigMapEnvFrom(name string) corev1.EnvFromSource {
	return corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
	}}
}

// edgeCreatePodWithDeletedServiceAccount creates the account and a Pod that
// uses it, then deletes the account. The label patch makes the Pod change
// after the deletion, so Kwatch sees the dangling reference.
func edgeCreatePodWithDeletedServiceAccount(
	ctx context.Context, e *harness.Environment, namespace, name string,
) error {
	core := e.Client.CoreV1()
	_, err := core.ServiceAccounts(namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{})
	if err != nil {
		return err
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PodSpec{
			ServiceAccountName: name,
			Containers:         []corev1.Container{edgeSleepContainer()},
		},
	}
	if err := edgeCreatePod(ctx, e, namespace, pod); err != nil {
		return err
	}
	err = core.ServiceAccounts(namespace).Delete(
		ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	label := []byte(`{"metadata":{"labels":{"kwatch-e2e":"` + name + `"}}}`)
	_, err = core.Pods(namespace).Patch(ctx, name,
		types.MergePatchType, label, metav1.PatchOptions{})
	return err
}

func edgeCreateIngressToMissingService(
	ctx context.Context, e *harness.Environment,
	namespace, name, service string,
) error {
	pathType := networkingv1.PathTypePrefix
	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: service,
			Port: networkingv1.ServiceBackendPort{Number: 8080},
		},
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType, Backend: backend,
				}},
			},
		}}},
	}
	_, err := e.Client.NetworkingV1().Ingresses(namespace).Create(
		ctx, ingress, metav1.CreateOptions{})
	return err
}

func edgeCreateDenyAllEgressPolicy(
	ctx context.Context, e *harness.Environment, namespace, name string,
) error {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
		},
	}
	_, err := e.Client.NetworkingV1().NetworkPolicies(namespace).Create(
		ctx, policy, metav1.CreateOptions{})
	return err
}

func edgeEnforceRestrictedSecurity(
	ctx context.Context, e *harness.Environment, namespace string,
) error {
	patch := []byte(`{"metadata":{"labels":{` +
		`"pod-security.kubernetes.io/enforce":"restricted"}}}`)
	_, err := e.Client.CoreV1().Namespaces().Patch(ctx, namespace,
		types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

func edgePrivilegedPod(name string) *corev1.Pod {
	privileged := true
	c := edgeSleepContainer()
	c.SecurityContext = &corev1.SecurityContext{Privileged: &privileged}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{c}},
	}
}
