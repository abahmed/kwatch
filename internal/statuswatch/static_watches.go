package statuswatch

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

// startStaticWatcher delegates built-in optional APIs to the shared dynamic
// watcher. CRD instance informers remain in crd.go because they are created
// and stopped as CRD versions change.
func (m *Monitor) startStaticWatcher(ctx context.Context) error {
	if m.staticWatcher == nil {
		return fmt.Errorf("statuswatch: static watcher is not configured")
	}
	generation, err := m.staticWatcher.StartGeneration(
		ctx, m.staticWatchSpecs(ctx),
	)
	if err == nil && generation.Valid() {
		m.mu.Lock()
		m.staticGeneration = generation
		m.mu.Unlock()
	}
	return err
}

func (m *Monitor) staticWatchSpecs(
	ctx context.Context,
) []dynamicwatch.ResourceSpec {
	specs := make([]dynamicwatch.ResourceSpec, 0,
		len(staticStatusWatches)+2)
	for _, watched := range staticStatusWatches {
		watched := watched
		if watched.resource == "endpoints" &&
			dynamicwatch.ResourceAvailableContext(
				ctx, m.discoveryClient, endpointSlicesGVR(),
			) {
			continue
		}
		specs = append(specs, dynamicwatch.ResourceSpec{
			GVR:        watched.gvr,
			Namespaced: watched.namespaced,
			Handlers: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					m.processStatic(obj, watched)
				},
				UpdateFunc: func(_, obj interface{}) {
					m.processStatic(obj, watched)
				},
				DeleteFunc: func(obj interface{}) {
					m.resolveStatic(obj, watched)
				},
			},
		})
	}
	specs = append(specs,
		m.admissionPolicySpec(), m.admissionBindingSpec())
	return specs
}

func endpointSlicesGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices",
	}
}

func (m *Monitor) admissionPolicySpec() dynamicwatch.ResourceSpec {
	return dynamicwatch.ResourceSpec{
		GVR: validatingAdmissionPolicyGVR,
		Handlers: cache.ResourceEventHandlerFuncs{
			AddFunc: m.processAdmissionPolicy,
			UpdateFunc: func(_, obj interface{}) {
				m.processAdmissionPolicy(obj)
			},
			DeleteFunc: m.deleteAdmissionPolicy,
		},
	}
}

func (m *Monitor) admissionBindingSpec() dynamicwatch.ResourceSpec {
	return dynamicwatch.ResourceSpec{
		GVR: validatingAdmissionBindingGVR,
		Handlers: cache.ResourceEventHandlerFuncs{
			AddFunc: m.processAdmissionBinding,
			UpdateFunc: func(_, obj interface{}) {
				m.processAdmissionBinding(obj)
			},
			DeleteFunc: m.deleteAdmissionBinding,
		},
	}
}
