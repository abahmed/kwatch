package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNewWiresEndpointSlicesForAdmissionWebhookMonitor(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AdmissionWebhookMonitor: config.AdmissionWebhookMonitor{Enabled: true},
	}

	ctrl, cleanup := newTestController(t, client, cfg, &mockHandler{})
	defer cleanup()

	assert.NotNil(t, ctrl.endpointSliceLister)
	assert.NotEmpty(t, ctrl.endpointSlice.synced)
	assert.False(t, ctrl.endpointSlice.startWorkers)
}

func TestEndpointSliceDeleteRequeuesServiceFromTombstone(t *testing.T) {
	ctrl := &Controller{
		now: func() time.Time { return time.Time{} },
		pipelineSet: pipelineSet{
			service:       newResourcePipeline("service", "services"),
			endpointSlice: newResourcePipeline("endpointslice", "endpointslices"),
		},
	}
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels: map[string]string{
				endpointSliceServiceLabel: "web",
			},
		},
	}
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "ns/web-hash",
		Obj: epSlice,
	}

	ctrl.endpointSliceEventHandler(true).DeleteFunc(tombstone)
	item, shutdown := ctrl.service.queue.Get()
	defer ctrl.service.queue.Done(item)
	defer ctrl.service.queue.ShutDown()

	assert.False(t, shutdown)
	assert.Equal(t, "ns/web", item)
}

func TestEndpointSliceEventRequeuesMatchingWebhooks(t *testing.T) {
	client := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(client, 0)
	mwcInformer := factory.Admissionregistration().V1().
		MutatingWebhookConfigurations()
	vwcInformer := factory.Admissionregistration().V1().
		ValidatingWebhookConfigurations()
	serviceRef := &admissionregistrationv1.ServiceReference{
		Namespace: "ns",
		Name:      "web",
	}
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: serviceRef,
			},
		}},
	}
	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: serviceRef,
			},
		}},
	}
	require.NoError(t, mwcInformer.Informer().GetIndexer().Add(mwc))
	require.NoError(t, vwcInformer.Informer().GetIndexer().Add(vwc))

	ctrl := &Controller{
		now: func() time.Time { return time.Time{} },
		pipelineSet: pipelineSet{
			mwc: newResourcePipeline("mwc", "mwc"),
			vwc: newResourcePipeline("vwc", "vwc"),
		},
		mwcLister: mwcInformer.Lister(),
		vwcLister: vwcInformer.Lister(),
	}
	ctrl.mwc.startWorkers = true
	ctrl.vwc.startWorkers = true
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels: map[string]string{
				endpointSliceServiceLabel: "web",
			},
		},
	}

	ctrl.endpointSliceEventHandler(false).AddFunc(epSlice)

	mwcItem, mwcShutdown := ctrl.mwc.queue.Get()
	vwcItem, vwcShutdown := ctrl.vwc.queue.Get()
	defer ctrl.mwc.queue.Done(mwcItem)
	defer ctrl.vwc.queue.Done(vwcItem)
	defer ctrl.mwc.queue.ShutDown()
	defer ctrl.vwc.queue.ShutDown()

	assert.False(t, mwcShutdown)
	assert.False(t, vwcShutdown)
	assert.Equal(t, "mwc", mwcItem)
	assert.Equal(t, "vwc", vwcItem)
}

func TestNewWithSingleNamespace(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"production"},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podLister)
}

func TestSyncEndpointSliceResolvesServiceByLabel(t *testing.T) {
	assert := assert.New(t)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Selector:  map[string]string{"app": "web"},
		},
	}
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels:    map[string]string{"kubernetes.io/service-name": "web"},
		},
	}
	client := fake.NewSimpleClientset(svc, epSlice)
	cfg := &config.Config{
		ServiceMonitor: config.ServiceMonitor{Enabled: true},
	}
	h := &mockHandler{}
	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	// The slice name ("web-hash") must not be looked up as the service name.
	err := ctrl.syncEndpointSlice(context.Background(), "ns/web-hash")
	assert.Nil(err)
}

func TestSyncEndpointSliceIgnoresUnlabeled(t *testing.T) {
	assert := assert.New(t)

	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "web-hash", Namespace: "ns"},
	}
	client := fake.NewSimpleClientset(epSlice)
	cfg := &config.Config{
		ServiceMonitor: config.ServiceMonitor{Enabled: true},
	}
	h := &mockHandler{}
	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncEndpointSlice(context.Background(), "ns/web-hash")
	assert.Nil(err)
}
