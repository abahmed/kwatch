package controller

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func (c *Controller) addPodToGraph(pod *corev1.Pod) {
	if c.graph == nil {
		return
	}
	ns := pod.Namespace
	name := pod.Name

	if pod.Spec.NodeName != "" {
		c.graph.AddEdge("pod", ns, name, "node", "", pod.Spec.NodeName, "scheduled_on")
	}
	for _, ref := range pod.OwnerReferences {
		ownerKind := strings.ToLower(ref.Kind)
		c.graph.AddEdge("pod", ns, name, ownerKind, ns, ref.Name, "owned_by")
	}
	for _, vol := range pod.Spec.Volumes {
		if cm := vol.ConfigMap; cm != nil {
			c.graph.AddEdge("pod", ns, name, "configmap", ns, cm.Name, "mounts")
		}
		if s := vol.Secret; s != nil {
			c.graph.AddEdge("pod", ns, name, "secret", ns, s.SecretName, "mounts")
		}
		if pvc := vol.PersistentVolumeClaim; pvc != nil {
			c.graph.AddEdge("pod", ns, name, "pvc", ns, pvc.ClaimName, "mounts")
		}
	}
	for _, ctr := range pod.Spec.Containers {
		c.addContainerEnvToGraph(ns, name, ctr)
	}
	for _, ctr := range pod.Spec.InitContainers {
		c.addContainerEnvToGraph(ns, name, ctr)
	}

	if c.serviceLister == nil {
		return
	}
	svcs, err := c.serviceLister.Services(ns).List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list services for graph edge", "namespace", ns)
		return
	}
	for _, svc := range svcs {
		if svc.Spec.Selector == nil {
			continue
		}
		if labels.SelectorFromSet(svc.Spec.Selector).Matches(labels.Set(pod.Labels)) {
			c.graph.AddEdge("service", ns, svc.Name, "pod", ns, name, "selects")
		}
	}
}

func (c *Controller) removePodFromGraph(pod *corev1.Pod) {
	if c.graph == nil {
		return
	}
	c.graph.RemoveNode("pod", pod.Namespace, pod.Name)
}

func (c *Controller) addContainerEnvToGraph(ns, podName string, ctr corev1.Container) {
	for _, envFrom := range ctr.EnvFrom {
		if cm := envFrom.ConfigMapRef; cm != nil {
			c.graph.AddEdge("pod", ns, podName, "configmap", ns, cm.Name, "env_from")
		}
		if s := envFrom.SecretRef; s != nil {
			c.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_from")
		}
	}
	for _, env := range ctr.Env {
		if env.ValueFrom != nil {
			if cm := env.ValueFrom.ConfigMapKeyRef; cm != nil {
				c.graph.AddEdge("pod", ns, podName, "configmap", ns, cm.Name, "env_ref")
			}
			if s := env.ValueFrom.SecretKeyRef; s != nil {
				c.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_ref")
			}
		}
	}
}

func (c *Controller) buildGraph() {
	if c.graph == nil {
		return
	}
	c.graph.Clear()

	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for graph build")
		return
	}
	for _, pod := range pods {
		c.addPodToGraph(pod)
	}

	klog.V(4).InfoS("dependency graph built from informer cache", "edges", len(c.graph.Edges()))
}

// pruneGraph performs mark-and-sweep on the resource graph: removes
// ConfigMap, Secret, and Service nodes that no longer exist in the
// informer cache. This prevents stale entries from accumulating
// between full rebuilds.
func (c *Controller) pruneGraph() {
	if c.graph == nil {
		return
	}

	active := make(map[string]bool)

	if cmLister := c.configMapLister; cmLister != nil {
		cms, err := cmLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list configmaps for graph pruning")
		} else {
			for _, cm := range cms {
				active["configmap/"+cm.Namespace+"/"+cm.Name] = true
			}
		}
	}

	if secretLister := c.secretLister; secretLister != nil {
		secrets, err := secretLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list secrets for graph pruning")
		} else {
			for _, s := range secrets {
				active["secret/"+s.Namespace+"/"+s.Name] = true
			}
		}
	}

	if svcLister := c.serviceLister; svcLister != nil {
		svcs, err := svcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list services for graph pruning")
		} else {
			for _, svc := range svcs {
				active["service/"+svc.Namespace+"/"+svc.Name] = true
			}
		}
	}

	pre := len(c.graph.Edges())
	c.graph.Prune("configmap", active)
	c.graph.Prune("secret", active)
	c.graph.Prune("service", active)
	post := len(c.graph.Edges())
	if pruned := pre - post; pruned > 0 {
		klog.V(4).InfoS("graph pruned", "removed", pruned, "remaining", post)
	}
}
