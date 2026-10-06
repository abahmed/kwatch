package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// bootEndpointScenarios are Services and a webhook whose backing pods
// are still starting on a freshly booted node pool.
func bootEndpointScenarios() []scenario {
	return []scenario{bootEndpointsQuiet(), bootEndpointsStillDown()}
}

// bootEndpointsQuiet: the pool boots and the Services in front of the
// new pods have no ready endpoints for four minutes, as does the
// fail-closed webhook served by the API. Everything is ready before the
// boot window ends, so nothing is said.
func bootEndpointsQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "boot-endpoints-quiet",
			Description: "A node pool boots; the Services and the " +
				"webhook backend have no ready endpoints for four " +
				"minutes while their pods start, then all are ready.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			web, api := bootWorkloads(c)
			bootEndpointObjects(c, web, api)
			bootPending(c, web, api)
			c.after(time.Minute)
			bootNodes(c)
			bootCreating(c, web, api)
			c.after(3 * time.Minute)
			bootReady(c, web, api, -1)
			c.update(bootService(c, "web", web, true)...)
			c.update(bootService(c, "api", api, true)...)
		},
	}
}

// bootEndpointsStillDown: the same boot, but the API pods start and
// never become ready. Once the boot window is over the API Service and
// the webhook it serves have still no endpoints, so they are reported.
func bootEndpointsStillDown() scenario {
	return scenario{
		expect: expectation{
			Name: "boot-endpoints-still-down",
			Description: "A node pool boots; the API pods start but " +
				"stay not ready, so its Service and the webhook it " +
				"serves still have no endpoints after the boot window.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			BootHeld:     true,
			MustNotBlame: []string{"nodepool//" + bootPool, "node//n1"},
			Tail:         duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			web, api := bootWorkloads(c)
			bootEndpointObjects(c, web, api)
			bootPending(c, web, api)
			c.after(time.Minute)
			bootNodes(c)
			bootCreating(c, web, api)
			c.after(3 * time.Minute)
			bootReady(c, web, api, 0)
			c.update(api.pod(1, "n2", startedNow, notReady))
			api.setReady(0)
			c.update(api.objects())
			c.update(bootService(c, "web", web, true)...)
		},
	}
}

// bootEndpointObjects lists the Services of both workloads, their
// endpoint slices and the webhook served by the API, none ready yet.
func bootEndpointObjects(c *cluster, web, api *workload) {
	var objects []runtime.Object
	objects = append(objects, bootService(c, "web", web, false)...)
	objects = append(objects, bootService(c, "api", api, false)...)
	hook := admissionMutatingHook(c, "policy", "policy", "shop", "api", 10)
	c.list(append(objects, hook)...)
}

// bootService is the Service of a workload and its endpoint slice, with
// an endpoint per pod that is ready only when ready is set.
func bootService(
	c *cluster, name string, w *workload, ready bool,
) []runtime.Object {
	pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n2")}
	slice := admissionSlice(c, "shop", name, 80, pods...)
	setEndpointsReady(slice, ready)
	return []runtime.Object{clusterService(c, "shop", name, 80), slice}
}

// setEndpointsReady sets the ready condition of every endpoint.
func setEndpointsReady(slice *discoveryv1.EndpointSlice, ready bool) {
	for i := range slice.Endpoints {
		slice.Endpoints[i].Conditions.Ready = boolPtr(ready)
	}
}
