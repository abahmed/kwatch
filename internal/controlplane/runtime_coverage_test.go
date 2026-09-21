package controlplane

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	corev1lister "k8s.io/client-go/listers/core/v1"
	restfake "k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestControlPlaneProbeHelpersAndStatusJSON(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	restClient := &restfake.RESTClient{
		NegotiatedSerializer: scheme.Codecs,
		GroupVersion:         corev1.SchemeGroupVersion,
		Resp: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(nilReader{}),
		},
	}
	sink := &controlPlaneSink{}
	monitor := NewWithRESTDependencies(
		restClient, fake.NewSimpleClientset(), config.ControlPlaneMonitor{},
		sink, resolverFunc(func(context.Context, string) ([]string, error) {
			return []string{"10.0.0.1"}, nil
		}), clock.Func(func() time.Time { return now }),
	)
	monitor.checkAPIServer(context.Background())
	monitor.checkCoreDNS(context.Background())
	monitor.markComponentsUnavailable(context.DeadlineExceeded)
	monitor.recordProbeError("test", context.Canceled)
	status := monitor.ControlPlaneStatus()
	if !status.APIServer.Available || !status.CoreDNS.Available {
		t.Fatalf("probe status = %#v", status)
	}
	if status.ProbeErrors != 1 {
		t.Fatalf("ProbeErrors = %d, want 1", status.ProbeErrors)
	}
	if _, err := monitor.StatusJSON(); err != nil {
		t.Fatalf("StatusJSON() error = %v", err)
	}
}

func TestControlPlaneProcessesPodsAndSweepsCache(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system", Name: "scheduler",
			Labels: map[string]string{"component": "kube-scheduler"},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff",
			}},
		}}},
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(pod); err != nil {
		t.Fatal(err)
	}
	sink := &controlPlaneSink{}
	monitor := &Monitor{incidentSink: sink}
	monitor.podLister = corev1lister.NewPodLister(indexer)
	monitor.ProcessControlPlanePod(nil)
	if err := monitor.ProcessControlPlanePod(&corev1.Pod{}); err != nil {
		t.Fatal(err)
	}
	if err := monitor.ProcessControlPlanePod(pod); err != nil {
		t.Fatal(err)
	}
	monitor.SweepControlPlane()
	if sink.processed != 2 {
		t.Fatalf("processed = %d, want 2", sink.processed)
	}
	if got, err := monitor.podLister.List(labels.Everything()); err != nil ||
		len(got) != 1 {
		t.Fatalf("pod lister result = %d, error = %v", len(got), err)
	}
}

func TestControlPlaneStartStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	monitor := NewWithRESTDependencies(
		&restfake.RESTClient{
			NegotiatedSerializer: scheme.Codecs,
			GroupVersion:         corev1.SchemeGroupVersion,
			Resp: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(nilReader{}),
			},
		},
		fake.NewSimpleClientset(), config.ControlPlaneMonitor{},
		&controlPlaneSink{}, resolverFunc(
			func(context.Context, string) ([]string, error) {
				return []string{"10.0.0.1"}, nil
			},
		), clock.RealClock{},
	)
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !monitor.started {
		t.Fatal("Start() did not mark monitor started")
	}
}

type resolverFunc func(context.Context, string) ([]string, error)

func (f resolverFunc) LookupHost(
	ctx context.Context, host string,
) ([]string, error) {
	return f(ctx, host)
}

type controlPlaneSink struct {
	processed int
}

func (s *controlPlaneSink) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed++
	return nil, model.ActionCreate
}

func (*controlPlaneSink) Resolve(model.ObjectRef, string) {}

func (*controlPlaneSink) ResolveObserved(*model.Observation) {}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
