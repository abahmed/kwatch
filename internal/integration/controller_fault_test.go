//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type recordingProvider struct {
	mu      sync.Mutex
	lastInc *model.Incident
	lastAct model.IncidentAction
	lastEv  *event.Event
}

func (p *recordingProvider) SendMessage(msg string) error { return nil }
func (p *recordingProvider) SendEvent(evt *event.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastEv = evt
	return nil
}
func (p *recordingProvider) Name() string { return "Recorder" }
func (p *recordingProvider) UsesEventDelivery() {}

func (p *recordingProvider) LastEvent() *event.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastEv
}

// TestControllerPodEvent verifies the end-to-end controller + handler pipeline
// for a pod with container issues. Asserts the incident is delivered with the
// correct action and dedup key.
func TestControllerPodEvent(t *testing.T) {
	cfg := &config.Config{
		App: config.App{DisableStartupMessage: true},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crash-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "my-dep", APIVersion: "apps/v1"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff 10s",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
					RestartCount: 3,
				},
			},
		},
	}

	client := fake.NewSimpleClientset(pod)

	correlator := correlation.NewEngine(correlation.Config{
		Window:            10 * time.Minute,
		LifecycleInterval: 1 * time.Minute,
		Enricher:          &enricher.DefaultEnricher{},
	})

	rec := &recordingProvider{}
	alertMgr := &alert.AlertManager{}
	alertMgr.Init(map[string]map[string]interface{}{}, &config.App{
		DisableStartupMessage: true,
	})
	alertMgr.AddProvider(rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	alertMgr.Start(ctx)

	h := handler.NewHandler(client, cfg, correlator, alertMgr)
	ctrl, cleanup := controller.New(client, cfg, h)
	defer cleanup()

	ctrlCtx, ctrlCancel := context.WithCancel(context.Background())
	defer ctrlCancel()

	var runErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = ctrl.Run(ctrlCtx, 1)
	}()

	time.Sleep(500 * time.Millisecond)

	err := h.ProcessPodObject(ctrlCtx, pod, false)
	if err != nil {
		t.Fatalf("ProcessPodObject failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	ctrlCancel()
	wg.Wait()
	if runErr != nil {
		t.Errorf("controller.Run returned error: %v", runErr)
	}

	// Assert incident was delivered via EventDeliveryProvider
	lastEv := rec.LastEvent()
	if lastEv == nil {
		t.Fatal("expected incident to be delivered to recording provider")
	}
	assert.Equal(t, "create", lastEv.Action, "first incident should be create action")
	assert.NotEmpty(t, lastEv.DedupKey, "incident must have dedup key")
	assert.Equal(t, "OOMKilled", lastEv.Reason)
}
