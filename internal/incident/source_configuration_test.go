package incident

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

type emptyAttributionSources struct{}

func (emptyAttributionSources) Deployment(
	string, string,
) (*appsv1.Deployment, error) {
	return nil, nil
}

func (emptyAttributionSources) StatefulSet(
	string, string,
) (*appsv1.StatefulSet, error) {
	return nil, nil
}

func (emptyAttributionSources) DaemonSet(
	string, string,
) (*appsv1.DaemonSet, error) {
	return nil, nil
}

func (emptyAttributionSources) Service(
	string, string,
) (*corev1.Service, error) {
	return nil, nil
}

func (emptyAttributionSources) ListServices(
	string,
) ([]*corev1.Service, error) {
	return nil, nil
}

func TestEngineConfiguresAttributionSourcesOnce(t *testing.T) {
	e := NewEngineWithClock(Config{}, clock.RealClock{})
	sources := emptyAttributionSources{}
	if err := e.ConfigureAttributionSources(sources); err != nil {
		t.Fatalf("configure sources: %v", err)
	}
	if err := e.ConfigureAttributionSources(sources); err == nil {
		t.Fatal("expected repeated source configuration to fail")
	}

	e.Resolve(model.ObjectRef{Kind: "pod", Name: "pod"}, "")
	if err := e.ConfigureAttributionSources(sources); err == nil {
		t.Fatal("expected source configuration after processing to fail")
	}
}
