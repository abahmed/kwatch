package controller

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (b *graphBuilder) rebuildService(obj interface{}) {
	if b.graph == nil {
		return
	}
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	if err := b.rebuildServiceChecked(svc); err != nil {
		klog.ErrorS(
			err, "failed to rebuild service graph edges; keeping previous edges",
			"namespace", svc.Namespace, "name", svc.Name,
		)
	}
}

func (b *graphBuilder) rebuildServiceChecked(svc *corev1.Service) error {
	if b.podLister == nil {
		return nil
	}
	if len(svc.Spec.Selector) == 0 {
		b.graph.ReplaceOutgoingEdges("service", svc.Namespace, svc.Name, nil)
		return nil
	}
	pods, err := b.podLister.Pods(svc.Namespace).List(
		labels.SelectorFromSet(svc.Spec.Selector),
	)
	if err != nil {
		return fmt.Errorf("list pods selected by service: %w", err)
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(pods))
	for _, pod := range pods {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "pod", Namespace: svc.Namespace,
			Name: pod.Name, Type: "selects",
		})
	}
	if b.endpointSliceLister != nil {
		slices, err := b.endpointSliceLister.EndpointSlices(
			svc.Namespace,
		).List(labels.Everything())
		if err != nil {
			return fmt.Errorf("list endpoint slices for service graph: %w", err)
		}
		for _, eps := range slices {
			if eps.Labels["kubernetes.io/service-name"] == svc.Name {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "endpointslice", Namespace: svc.Namespace,
					Name: eps.Name, Type: graphEdgeProvides,
				})
			}
		}
	}
	b.graph.ReplaceOutgoingEdges(
		"service", svc.Namespace, svc.Name, targets,
	)
	return nil
}

func (b *graphBuilder) rebuildReplicaSet(obj interface{}) {
	if b.graph == nil {
		return
	}
	rs, ok := obj.(*appsv1.ReplicaSet)
	if !ok {
		return
	}
	b.graph.ReplaceOutgoingEdges(
		"replicaset", rs.Namespace, rs.Name,
		ownedByTargets(rs.Namespace, rs.OwnerReferences),
	)
}

func (b *graphBuilder) rebuildJob(obj interface{}) {
	if b.graph == nil {
		return
	}
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return
	}
	b.graph.ReplaceOutgoingEdges(
		"job", job.Namespace, job.Name,
		ownedByTargets(job.Namespace, job.OwnerReferences),
	)
}

func (b *graphBuilder) rebuildIngress(obj interface{}) {
	if b.graph == nil {
		return
	}
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0)
	if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "ingressclass", Name: *ing.Spec.IngressClassName,
			Type: "uses_class",
		})
	}
	add := func(svc *networkingv1.IngressServiceBackend) {
		if svc != nil && svc.Name != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "service", Namespace: ing.Namespace,
				Name: svc.Name, Type: graphEdgeRoutesTo,
			})
		}
	}
	if ing.Spec.DefaultBackend != nil {
		add(ing.Spec.DefaultBackend.Service)
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			add(path.Backend.Service)
		}
	}
	for _, tls := range ing.Spec.TLS {
		if tls.SecretName != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "secret", Namespace: ing.Namespace,
				Name: tls.SecretName, Type: graphEdgeTLS,
			})
		}
	}
	b.graph.ReplaceOutgoingEdges(
		"ingress", ing.Namespace, ing.Name, targets,
	)
}

func (b *graphBuilder) rebuildHorizontalPodAutoscaler(obj interface{}) {
	if b.graph == nil {
		return
	}
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return
	}
	var targets []kwcontext.EdgeTarget
	ref := hpa.Spec.ScaleTargetRef
	if ref.Name != "" && ref.Kind != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: strings.ToLower(ref.Kind), Namespace: hpa.Namespace,
			Name: ref.Name, Type: graphEdgeScales,
		})
	}
	b.graph.ReplaceOutgoingEdges(
		"horizontalpodautoscaler", hpa.Namespace, hpa.Name, targets,
	)
}
