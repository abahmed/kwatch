package kube

import (
	"context"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// endpointFetchBudget bounds one endpoint read for an investigation.
	endpointFetchBudget = 2 * time.Second
	// maxEndpointSlices bounds the slices read for one Service.
	maxEndpointSlices = 50
)

// EndpointReader counts a Service's ready endpoints straight from the
// API. Investigations use it only when the model has no slices for the
// Service, for example while EndpointSlices are not watched.
type EndpointReader struct {
	Client kubernetes.Interface
}

// ReadyEndpoints returns the ready endpoints of service across its
// EndpointSlices. ok is false when the read failed or service is not a
// Service, so a missing answer is never reported as zero endpoints.
func (r EndpointReader) ReadyEndpoints(
	ctx context.Context, service inventory.EntityID,
) (int, bool) {
	if r.Client == nil || service.Kind != KindService {
		return 0, false
	}
	readCtx, cancel := context.WithTimeout(ctx, endpointFetchBudget)
	defer cancel()
	list, err := r.Client.DiscoveryV1().EndpointSlices(service.Namespace).
		List(readCtx, metav1.ListOptions{
			LabelSelector: discoveryv1.LabelServiceName + "=" + service.Name,
			Limit:         maxEndpointSlices,
		})
	if err != nil {
		return 0, false
	}
	ready := 0
	for _, slice := range list.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				ready++
			}
		}
	}
	return ready, true
}
