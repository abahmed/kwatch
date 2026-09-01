package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

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
