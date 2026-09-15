package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type mockServiceLister struct {
	corev1lister.ServiceLister
	listFn func(ns string) ([]*corev1.Service, error)
}

func (m *mockServiceLister) Services(
	namespace string,
) corev1lister.ServiceNamespaceLister {
	return &mockSvcNsLister{listFn: func() ([]*corev1.Service, error) {
		return m.listFn(namespace)
	}}
}

type mockSvcNsLister struct {
	corev1lister.ServiceNamespaceLister
	listFn func() ([]*corev1.Service, error)
}

func (m *mockSvcNsLister) List(
	selector labels.Selector,
) ([]*corev1.Service, error) {
	return m.listFn()
}

func (m *mockSvcNsLister) Get(name string) (*corev1.Service, error) {
	return nil, nil
}

func TestFindDependentServicesNoLister(t *testing.T) {
	e := newTestEngine()
	got := dependentServices(
		e.attributionSources, "ns", map[string]string{"app": "myapp"},
	)
	assert.Nil(t, got)
}

func TestFindDependentServicesNoLabels(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{})
	got := dependentServices(e.attributionSources, "ns", nil)
	assert.Nil(t, got)
}

func TestFindDependentServicesMatch(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
			}, nil
		},
	})
	got := dependentServices(
		e.attributionSources, "ns", map[string]string{"app": "api"},
	)
	assert.Equal(t, []string{"svc-api"}, got)
}

func TestFindDependentServicesNoMatch(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
			}, nil
		},
	})
	got := dependentServices(
		e.attributionSources, "ns", map[string]string{"app": "web"},
	)
	assert.Empty(t, got)
}

func TestFindDependentServicesMultiple(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-api",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-grpc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "api"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-other",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "other"},
					},
				},
			}, nil
		},
	})
	got := dependentServices(
		e.attributionSources, "ns", map[string]string{"app": "api"},
	)
	assert.Len(t, got, 2)
	assert.Contains(t, got, "svc-api")
	assert.Contains(t, got, "svc-grpc")
}

func TestFindDependentServicesEmptySelector(t *testing.T) {
	e := newTestEngine()
	e.SetServiceLister(&mockServiceLister{
		listFn: func(ns string) ([]*corev1.Service, error) {
			return []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-headless",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{Selector: nil},
				},
			}, nil
		},
	})
	got := dependentServices(
		e.attributionSources, "ns", map[string]string{"app": "api"},
	)
	assert.Empty(t, got)
}
