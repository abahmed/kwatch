package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

var defaultServiceSustainedSeconds float64 = 60

func DetectServiceEndpointIssue(
	svc *corev1.Service,
	epSlices []*discoveryv1.EndpointSlice,
) *event.Signal {
	if svc == nil {
		return nil
	}
	if svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
		return nil
	}
	if svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" {
		return nil
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	hasReady := false
	for _, slice := range epSlices {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				hasReady = true
				break
			}
		}
		if hasReady {
			break
		}
	}

	if !hasReady {
		key := svc.Namespace + "/" + svc.Name
		return &event.Signal{
			Resource:  "service",
			Namespace: svc.Namespace,
			Reason:    constant.ReasonServiceNoEndpoints,
			Owner:     key,
			PodName:   svc.Name,
			Labels:    svc.Labels,
			Hint: fmt.Sprintf(
				"service %s has selectors but no ready endpoints",
				key,
			),
		}
	}
	return nil
}

func (h *handler) ProcessService(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid service key %q: %w", key, err)
	}
	if deleted {
		h.correlator.ResolveByResource("service", namespace+"/"+name)
		return nil
	}
	svc, err := h.listers.Service.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("service", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf(
			"failed to get service %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}
	return h.ProcessServiceObject(svc, false)
}

func (h *handler) ProcessServiceObject(
	svc *corev1.Service,
	deleted bool,
) error {
	if svc == nil {
		return nil
	}

	if deleted {
		h.clearServiceNoEndpoints(svc.Namespace, svc.Name)
		h.correlator.ResolveByResource("service", svc.Namespace+"/"+svc.Name)
		return nil
	}

	sel := labels.Set{"kubernetes.io/service-name": svc.Name}.AsSelector()
	epSlices, err := h.listers.EndpointSlice.EndpointSlices(
		svc.Namespace,
	).List(
		sel,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to list endpoint slices for %s/%s: %w",
			svc.Namespace,
			svc.Name,
			err,
		)
	}

	sig := DetectServiceEndpointIssue(svc, epSlices)
	key := svc.Namespace + "/" + svc.Name
	if sig != nil {
		first := h.markServiceNoEndpoints(key)
		sustained := time.Duration(defaultServiceSustainedSeconds) * time.Second
		if h.now().Sub(first) < sustained {
			return nil
		}
		h.signalEvent(sig)
	} else {
		h.clearServiceNoEndpoints(svc.Namespace, svc.Name)
		h.correlator.MarkResolved(
			correlation.BuildKey(
				svc.Namespace,
				svc.Namespace+"/"+svc.Name,
				constant.ReasonServiceNoEndpoints,
				"",
			),
		)
	}
	return nil
}

func (h *handler) markServiceNoEndpoints(key string) time.Time {
	return h.fs.serviceNoEndpoint.mark(key, h.now())
}

func (h *handler) clearServiceNoEndpoints(namespace, name string) {
	h.fs.serviceNoEndpoint.clear(correlation.OwnerPath(namespace, name))
}
