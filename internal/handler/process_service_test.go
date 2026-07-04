package handler

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProcessServiceCreatesIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)
	f.Core().V1().Endpoints().Informer().GetIndexer().Add(eps)
	h.SetServiceLister(f.Core().V1().Services().Lister())
	h.SetEndpointLister(f.Core().V1().Endpoints().Lister())

	assert.NoError(t, h.ProcessService("default/test-svc", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "ServiceNoEndpoints" && v.State == model.StateActive {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessService should create incident")
}

func TestProcessServiceKeyDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessService("default/test-svc", true))
}

func TestProcessServiceKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetServiceLister(f.Core().V1().Services().Lister())

	assert.NoError(t, h.ProcessService("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestDetectServiceEndpointIssueNoSelector(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.Nil(t, sig, "service without selectors should not produce a signal")
}

func TestDetectServiceEndpointIssueHeadless(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  map[string]string{"app": "test"},
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.Nil(t, sig, "headless service should not produce a signal")
}

func TestDetectServiceEndpointIssueExternalName(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:       corev1.ServiceTypeExternalName,
			Selector:   map[string]string{"app": "test"},
			ClusterIP:  "",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.Nil(t, sig, "ExternalName service should not produce a signal")
}

func TestDetectServiceEndpointIssueReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}}},
		},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.Nil(t, sig, "service with ready endpoints should not produce a signal")
}

func TestDetectServiceEndpointIssueNoReadyEndpoints(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.NotNil(t, sig)
	assert.Equal(t, "ServiceNoEndpoints", sig.Reason)
	assert.Equal(t, "service", sig.Resource)
	assert.Equal(t, "default/test-svc", sig.Owner)
}

func TestDetectServiceEndpointIssueOnlyNotReadyAddresses(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{NotReadyAddresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}}},
		},
	}
	sig := DetectServiceEndpointIssue(svc, eps)
	assert.NotNil(t, sig, "service with only not-ready addresses should produce a signal")
}

func TestProcessServiceObjectNoEndpointsIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Endpoints().Informer().GetIndexer().Add(eps)
	h.SetEndpointLister(f.Core().V1().Endpoints().Lister())

	assert.NoError(t, h.ProcessServiceObject(svc, false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "ServiceNoEndpoints" {
			found = true
			assert.Equal(t, model.StateActive, v.State)
		}
	}
	assert.True(t, found, "ServiceNoEndpoints incident should exist")
}

func TestProcessServiceObjectResolve(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	indexer := f.Core().V1().Endpoints().Informer().GetIndexer()

	epsNoReady := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	indexer.Add(epsNoReady)
	h.SetEndpointLister(f.Core().V1().Endpoints().Lister())

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(t, 1, e.ActiveCount(), "incident should be created for no endpoints")

	indexer.Delete(epsNoReady)
	epsReady := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}}},
		},
	}
	indexer.Add(epsReady)

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(t, 0, e.ActiveCount(), "incident should be resolved when endpoints are ready")
}

func TestProcessServiceObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessServiceObject(svc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessServiceObjectNil(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessServiceObject(nil, false))
}

func TestProcessServiceObjectNoSelectorNoIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Endpoints().Informer().GetIndexer().Add(eps)
	h.SetEndpointLister(f.Core().V1().Endpoints().Lister())

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(t, 0, e.ActiveCount(), "service without selectors should not create incident")
}
