package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func epSliceForSvc(name, ns string, ready bool) *discoveryv1.EndpointSlice {
	eps := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-slice",
			Namespace: ns,
			Labels:    map[string]string{"kubernetes.io/service-name": name},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.0.0.2"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &ready,
				},
			},
		},
	}
	return eps
}

func emptyEpSlice(name, ns string) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-slice",
			Namespace: ns,
			Labels:    map[string]string{"kubernetes.io/service-name": name},
		},
	}
}

func TestProcessServiceCreatesIncident(t *testing.T) {
	defaultServiceSustainedSeconds = 0
	defer func() { defaultServiceSustainedSeconds = 60 }()

	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)
	f.Discovery().V1().EndpointSlices().Informer().GetIndexer().Add(
		emptyEpSlice("test-svc", "default"),
	)
	h.listers.Service = f.Core().V1().Services().Lister()
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

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
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessService("default/test-svc", true))
}

func TestProcessServiceKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Service = f.Core().V1().Services().Lister()

	assert.NoError(t, h.ProcessService("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestDetectServiceEndpointIssueNoSelector(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	sig := DetectServiceEndpointIssue(svc, nil)
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
	sig := DetectServiceEndpointIssue(svc, nil)
	assert.Nil(t, sig, "headless service should not produce a signal")
}

func TestDetectServiceEndpointIssueExternalName(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeExternalName,
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "192.168.1.1",
		},
	}
	sig := DetectServiceEndpointIssue(svc, nil)
	assert.Nil(t, sig, "ExternalName service should not produce a signal")
}

func TestDetectServiceEndpointIssueReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	epSlices := []*discoveryv1.EndpointSlice{
		epSliceForSvc("test-svc", "default", true),
	}
	sig := DetectServiceEndpointIssue(svc, epSlices)
	assert.Nil(
		t,
		sig,
		"service with ready endpoints should not produce a signal",
	)
}

func TestDetectServiceEndpointIssueNoReadyEndpoints(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	epSlices := []*discoveryv1.EndpointSlice{
		emptyEpSlice("test-svc", "default"),
	}
	sig := DetectServiceEndpointIssue(svc, epSlices)
	assert.NotNil(t, sig)
	assert.Equal(t, "ServiceNoEndpoints", sig.Reason)
	assert.Equal(t, "service", sig.Resource)
	assert.Equal(t, "default/test-svc", sig.Owner)
}

func TestDetectServiceEndpointIssueOnlyNotReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	epSlices := []*discoveryv1.EndpointSlice{
		epSliceForSvc("test-svc", "default", false),
	}
	sig := DetectServiceEndpointIssue(svc, epSlices)
	assert.NotNil(
		t,
		sig,
		"service with only not-ready addresses should produce a signal",
	)
}

func TestDetectServiceEndpointIssueMultipleNotReady(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	notReady := false
	epSlices := []*discoveryv1.EndpointSlice{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "test-svc",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{"10.0.0.2"},
				Conditions: discoveryv1.EndpointConditions{Ready: &notReady},
			},
		},
	}}
	sig := DetectServiceEndpointIssue(svc, epSlices)
	assert.NotNil(
		t,
		sig,
		"service with only not-ready endpoint conditions should produce a "+
			"signal",
	)
}

func TestProcessServiceObjectNoEndpointsIncident(t *testing.T) {
	defaultServiceSustainedSeconds = 0
	defer func() { defaultServiceSustainedSeconds = 60 }()

	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	sel := labels.SelectorFromSet(
		map[string]string{"kubernetes.io/service-name": "test-svc"},
	)
	_ = sel
	f.Discovery().V1().EndpointSlices().Informer().GetIndexer().Add(
		emptyEpSlice("test-svc", "default"),
	)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

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
	defaultServiceSustainedSeconds = 0
	defer func() { defaultServiceSustainedSeconds = 60 }()

	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	indexer := f.Discovery().V1().EndpointSlices().Informer().GetIndexer()

	epNoReady := emptyEpSlice("test-svc", "default")
	indexer.Add(epNoReady)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(
		t,
		1,
		e.ActiveCount(),
		"incident should be created for no endpoints",
	)

	indexer.Delete(epNoReady)
	epReady := epSliceForSvc("test-svc", "default", true)
	indexer.Add(epReady)

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"incident should be resolved when endpoints are ready",
	)
}

func TestProcessServiceObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessServiceObject(svc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessServiceObjectNil(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessServiceObject(nil, false))
}

func TestProcessServiceInvalidKey(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.Error(t, h.ProcessService("a/b/c", false))
}

func TestProcessServiceObjectSliceNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1"},
	}

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"endpoints not found should not create incident",
	)
}

func TestProcessServiceObjectClusterIPEmpty(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Discovery().V1().EndpointSlices().Informer().GetIndexer().Add(
		emptyEpSlice("test-svc", "default"),
	)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"service with empty ClusterIP should not create incident",
	)
}

func TestProcessServiceObjectNoSelectorNoIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Discovery().V1().EndpointSlices().Informer().GetIndexer().Add(
		emptyEpSlice("test-svc", "default"),
	)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()

	assert.NoError(t, h.ProcessServiceObject(svc, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"service without selectors should not create incident",
	)
}

func TestDetectServiceEndpointIssueEmptySlice(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	sig := DetectServiceEndpointIssue(svc, []*discoveryv1.EndpointSlice{})
	assert.NotNil(t, sig, "empty slice list should produce a signal")
}

func TestDetectServiceEndpointIssueNilSlice(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	sig := DetectServiceEndpointIssue(svc, nil)
	assert.NotNil(t, sig, "nil slices should produce a signal")
}

func TestDetectServiceEndpointIssueReadyInSecondSlice(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "test"},
			ClusterIP: "10.0.0.1",
		},
	}
	notReady := false
	ready := true
	epSlices := []*discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a",
				Namespace: "default",
				Labels: map[string]string{
					"kubernetes.io/service-name": "test-svc",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &notReady,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "b",
				Namespace: "default",
				Labels: map[string]string{
					"kubernetes.io/service-name": "test-svc",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.3"},
					Conditions: discoveryv1.EndpointConditions{Ready: &ready},
				},
			},
		},
	}
	sig := DetectServiceEndpointIssue(svc, epSlices)
	assert.Nil(
		t,
		sig,
		"ready endpoint in second slice should not produce a signal",
	)
}
