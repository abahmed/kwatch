package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// policyAdmissionScenarios are built-in admission checks (Pod Security
// Admission, LimitRange, ValidatingAdmissionPolicy) that refuse the pods
// a controller creates.
func policyAdmissionScenarios() []scenario {
	return []scenario{podSecurityRejects(), limitRangeRejects(),
		limitRangeMemoryRejects(), admissionPolicyDenies()}
}

// policyDenied runs a Deployment of two pods, lets setup change the
// cluster, then scales the Deployment up so the new pods are refused
// with message (which gets the pod name).
func policyDenied(c *cluster, namespace, name string,
	setup func(c *cluster), message string) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment(namespace, name, "registry.example.com/"+name+":2", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	c.after(3 * time.Minute)
	setup(c)
	c.after(time.Minute)
	setReplicas(w, 4)
	w.setReady(2)
	c.update(w.objects())
	for n := int32(1); n <= 5; n++ {
		text := fmt.Sprintf("Error creating: pods \"%s-%c\" is "+
			"forbidden: %s", w.replicaSet.Name, 'e'+rune(n), message)
		c.warn(c.warningEvent(w.replicaSet, "ReplicaSet", "FailedCreate",
			text, "replicaset-controller", n*2))
		c.after(time.Minute)
	}
}

// namespaceOf is the Namespace object with labels.
func namespaceOf(c *cluster, name string,
	labels map[string]string) *corev1.Namespace {
	meta := clusterMeta(c, "", name, "namespace")
	meta.Labels = labels
	return &corev1.Namespace{ObjectMeta: meta,
		Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
}

// podSecurityRejects: a platform team labels the namespace of the
// reports service with an enforced restricted profile; the next scale-up
// is refused by Pod Security Admission. The namespace is the root.
func podSecurityRejects() scenario {
	const label = "pod-security.kubernetes.io/enforce"
	return scenario{
		expect: expectation{
			Name: "pod-security-enforce-label-added",
			Description: "A namespace gets an enforced restricted Pod " +
				"Security label; a scale-up of its Deployment is " +
				"refused with \"violates PodSecurity\".",
			Root: "namespace//reports", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/reports/api"},
		},
		build: func(c *cluster) {
			c.list(namespaceOf(c, "reports", nil))
			policyDenied(c, "reports", "api", func(c *cluster) {
				c.update(namespaceOf(c, "reports",
					map[string]string{label: "restricted"}))
			}, "violates PodSecurity \"restricted:latest\": "+
				"allowPrivilegeEscalation != false (container \"api\" "+
				"must set securityContext.allowPrivilegeEscalation=false)")
		},
	}
}

// limitRangeRejects: the LimitRange of the billing namespace is edited
// to cap a container at one CPU; the next pods, which ask for two, are
// refused. The LimitRange is the root.
func limitRangeRejects() scenario {
	return scenario{
		expect: expectation{
			Name: "limit-range-max-lowered",
			Description: "A LimitRange is edited to cap containers at " +
				"one CPU; a scale-up of a Deployment whose pods limit " +
				"two is refused.",
			Root: "limitrange/billing/container-bounds", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/billing/api",
				"namespace//billing"},
		},
		build: func(c *cluster) {
			bounds := func(cpu string) *corev1.LimitRange {
				return &corev1.LimitRange{
					ObjectMeta: clusterMeta(c, "billing",
						"container-bounds", "limitrange"),
					Spec: corev1.LimitRangeSpec{Limits: []corev1.
						LimitRangeItem{{Type: corev1.LimitTypeContainer,
						Max: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse(cpu)}}}},
				}
			}
			c.list(bounds("4"))
			policyDenied(c, "billing", "api", func(c *cluster) {
				c.update(bounds("1"))
			}, "maximum cpu usage per Container is 1, but limit is 2")
		},
	}
}

// admissionPolicyDenies: a ValidatingAdmissionPolicy that requires a
// team label was bound to the mailing namespace; the pods of the next
// scale-up lack it and the API server names the policy in its denial.
func admissionPolicyDenies() scenario {
	return scenario{
		expect: expectation{
			Name: "admission-policy-denies-pods",
			Description: "A new ValidatingAdmissionPolicy denies pods " +
				"without a team label; a scale-up of a Deployment is " +
				"refused and the denial names the policy.",
			Root: "validatingadmissionpolicy//require-team.example.com",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/mailing/relay"},
		},
		build: func(c *cluster) {
			policyDenied(c, "mailing", "relay", func(c *cluster) {
				c.create(admissionPolicy(c, "require-team.example.com"))
			}, "ValidatingAdmissionPolicy 'require-team.example.com' "+
				"with binding 'require-team-binding' denied request: "+
				"pods must carry the team label")
		},
	}
}

// admissionPolicy is a cluster-scoped ValidatingAdmissionPolicy.
func admissionPolicy(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "", name, "policy")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingAdmissionPolicy",
		"spec": map[string]any{"failurePolicy": "Fail",
			"validations": []any{map[string]any{
				"expression": "has(object.metadata.labels.team)"}}},
	}}
	u.SetName(meta.Name)
	u.SetUID(meta.UID)
	return u
}
