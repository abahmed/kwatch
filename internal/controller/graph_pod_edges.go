package controller

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func (c *Controller) addPodToGraph(pod *corev1.Pod) {
	if err := c.newGraphBuilder(c.graph).addPodToGraphChecked(pod); err != nil {
		klog.ErrorS(
			err, "failed to build pod graph edges",
			"namespace", pod.Namespace, "pod", pod.Name,
		)
	}
}

func (b *graphBuilder) addPodToGraphChecked(pod *corev1.Pod) error {
	if b.graph == nil {
		return nil
	}
	ns := pod.Namespace
	name := pod.Name

	if pod.Spec.NodeName != "" {
		b.graph.AddEdge("pod", ns, name, "node", "", pod.Spec.NodeName,
			"scheduled_on")
	}
	if pod.Spec.ServiceAccountName != "" {
		b.graph.AddEdge("pod", ns, name, "serviceaccount", ns,
			pod.Spec.ServiceAccountName, "uses_sa")
	}
	for _, secret := range pod.Spec.ImagePullSecrets {
		if secret.Name != "" {
			b.graph.AddEdge("pod", ns, name, "secret", ns, secret.Name,
				graphEdgeUsesPull)
		}
	}
	for _, ref := range pod.OwnerReferences {
		ownerKind := strings.ToLower(ref.Kind)
		b.graph.AddEdge("pod", ns, name, ownerKind, ns, ref.Name, "owned_by")
	}
	for _, vol := range pod.Spec.Volumes {
		b.addPodVolumeToGraph(ns, name, vol)
	}
	for _, ctr := range pod.Spec.Containers {
		b.addContainerEnvToGraph(ns, name, ctr)
	}
	for _, ctr := range pod.Spec.InitContainers {
		b.addContainerEnvToGraph(ns, name, ctr)
	}

	if b.serviceLister == nil {
		return nil
	}
	svcs, err := b.serviceLister.Services(ns).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list services for graph edge: %w", err)
	}
	for _, svc := range svcs {
		if svc.Spec.Selector == nil {
			continue
		}
		if labels.SelectorFromSet(svc.Spec.Selector).Matches(
			labels.Set(pod.Labels),
		) {
			b.graph.AddEdge("service", ns, svc.Name, "pod", ns, name, "selects")
		}
	}
	return nil
}

// clusterCABundles are projected into every Pod's service-account volume and
// do not identify a workload, so they are intentionally omitted from RCA.
var clusterCABundles = map[string]bool{
	"kube-root-ca.crt":         true,
	"openshift-service-ca.crt": true,
}

func (b *graphBuilder) addConfigMapEdge(ns, podName, name, edgeType string) {
	if name == "" || clusterCABundles[name] {
		return
	}
	b.graph.AddEdge("pod", ns, podName, "configmap", ns, name, edgeType)
}

func (b *graphBuilder) addPodVolumeToGraph(
	ns, podName string, vol corev1.Volume,
) {
	if cm := vol.ConfigMap; cm != nil {
		b.addConfigMapEdge(ns, podName, cm.Name, "mounts")
	}
	if secret := vol.Secret; secret != nil {
		b.graph.AddEdge("pod", ns, podName, "secret", ns, secret.SecretName,
			"mounts")
	}
	if pvc := vol.PersistentVolumeClaim; pvc != nil {
		b.graph.AddEdge("pod", ns, podName, "pvc", ns, pvc.ClaimName, "mounts")
	}
	if projected := vol.Projected; projected != nil {
		for _, source := range projected.Sources {
			if source.ConfigMap != nil {
				b.addConfigMapEdge(
					ns, podName, source.ConfigMap.Name, graphEdgeProjects,
				)
			}
			if source.Secret != nil {
				b.graph.AddEdge("pod", ns, podName, "secret", ns,
					source.Secret.Name, graphEdgeProjects)
			}
		}
	}
	if csi := vol.CSI; csi != nil && csi.Driver != "" {
		b.graph.AddEdge("pod", ns, podName, "csidriver", "", csi.Driver,
			graphEdgeUsesCSI)
	}
}

func (b *graphBuilder) addContainerEnvToGraph(
	ns, podName string, ctr corev1.Container,
) {
	for _, envFrom := range ctr.EnvFrom {
		if cm := envFrom.ConfigMapRef; cm != nil {
			b.addConfigMapEdge(ns, podName, cm.Name, "env_from")
		}
		if s := envFrom.SecretRef; s != nil {
			b.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_from")
		}
	}
	for _, env := range ctr.Env {
		if env.ValueFrom == nil {
			continue
		}
		if cm := env.ValueFrom.ConfigMapKeyRef; cm != nil {
			b.addConfigMapEdge(ns, podName, cm.Name, "env_ref")
		}
		if s := env.ValueFrom.SecretKeyRef; s != nil {
			b.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_ref")
		}
	}
}
