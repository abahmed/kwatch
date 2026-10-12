//go:build e2e

package scenarios

import (
	"context"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// deniedPodLabel marks the Pods the policy rejects, so Pods of other
	// workloads in the same namespace stay admitted.
	deniedPodLabel = "kwatch-e2e/admission"
	// policyEffectTimeout bounds how long the API server may need to
	// start enforcing a new policy.
	policyEffectTimeout = 2 * time.Minute
)

// denyPolicyName names the policy and binding of one scenario. They are
// cluster-scoped, so the namespace keeps parallel scenarios apart.
func denyPolicyName(namespace string) string {
	return "kwatch-e2e-deny-" + namespace
}

// denyLabeledPodsPolicy rejects the creation of every Pod that carries
// deniedPodLabel.
func denyLabeledPodsPolicy(
	name string,
) *admissionregistrationv1.ValidatingAdmissionPolicy {
	return &admissionregistrationv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: ptr(admissionregistrationv1.Fail),
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.
					NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.
						RuleWithOperations{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				}},
			},
			Validations: []admissionregistrationv1.Validation{{
				Expression: "false",
				Message:    "pods are denied by the kwatch e2e policy",
			}},
		},
	}
}

// denyLabeledPodsBinding applies the policy to the labelled Pods of one
// namespace and denies them.
func denyLabeledPodsBinding(
	name, namespace string,
) *admissionregistrationv1.ValidatingAdmissionPolicyBinding {
	return &admissionregistrationv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: admissionregistrationv1.
			ValidatingAdmissionPolicyBindingSpec{
			PolicyName: name,
			ValidationActions: []admissionregistrationv1.ValidationAction{
				admissionregistrationv1.Deny},
			MatchResources: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": namespace},
				},
				ObjectSelector: &metav1.LabelSelector{
					MatchLabels: deniedLabels()},
			},
		},
	}
}

func deniedLabels() map[string]string {
	return map[string]string{deniedPodLabel: "deny"}
}

// deniedReplicaSet is healthy, but its Pods carry deniedPodLabel.
func deniedReplicaSet(name string) *appsv1.ReplicaSet {
	set := healthyReplicaSet(name)
	for key, value := range deniedLabels() {
		set.Spec.Template.Labels[key] = value
	}
	return set
}

// DenyLabeledPods installs the policy and binding for the scenario
// namespace and waits until the API server enforces them, so a workload
// created afterwards cannot race the policy. The returned function lifts
// the denial; it also runs when the test ends.
func (s *Scenario) DenyLabeledPods() (lift func()) {
	s.T.Helper()
	name := denyPolicyName(s.Namespace)
	client := s.Env.Client.AdmissionregistrationV1()
	_, err := client.ValidatingAdmissionPolicies().Create(s.Ctx,
		denyLabeledPodsPolicy(name), metav1.CreateOptions{})
	s.Must(err)
	_, err = client.ValidatingAdmissionPolicyBindings().Create(s.Ctx,
		denyLabeledPodsBinding(name, s.Namespace), metav1.CreateOptions{})
	s.Must(err)
	lift = func() { removeDenyPolicy(s, name) }
	s.T.Cleanup(lift)
	s.waitForDenial(name)
	return lift
}

// waitForDenial probes with a labelled Pod until the API server rejects
// it with the policy's own message.
func (s *Scenario) waitForDenial(policy string) {
	s.T.Helper()
	probe := workloadPod("probe", "sleep")
	probe.Labels = deniedLabels()
	err := wait.PollUntilContextTimeout(s.Ctx, time.Second,
		policyEffectTimeout, true, func(ctx context.Context) (bool, error) {
			_, err := s.Env.Client.CoreV1().Pods(s.Namespace).Create(
				ctx, probe, metav1.CreateOptions{})
			if err == nil {
				// Not enforced yet: drop the probe and ask again.
				_ = s.Env.Client.CoreV1().Pods(s.Namespace).Delete(
					ctx, probe.Name, metav1.DeleteOptions{})
				return false, nil
			}
			if !apierrors.IsForbidden(err) {
				return false, err
			}
			return strings.Contains(err.Error(), policy), nil
		})
	s.Must(err)
}

// removeDenyPolicy deletes the binding and the policy; absent objects are
// fine, so lifting twice is safe.
func removeDenyPolicy(s *Scenario, name string) {
	ctx, cancel := context.WithTimeout(
		context.Background(), cleanupTimeout)
	defer cancel()
	client := s.Env.Client.AdmissionregistrationV1()
	_ = client.ValidatingAdmissionPolicyBindings().Delete(
		ctx, name, metav1.DeleteOptions{})
	_ = client.ValidatingAdmissionPolicies().Delete(
		ctx, name, metav1.DeleteOptions{})
}

// podsOwnedBy reports whether any Pod's name starts with the ReplicaSet's.
func podsOwnedBy(pods []corev1.Pod, replicaSet string) bool {
	for i := range pods {
		if strings.HasPrefix(pods[i].Name, replicaSet+"-") {
			return true
		}
	}
	return false
}
