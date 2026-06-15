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
	"github.com/abahmed/kwatch/internal/handler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestControllerPodEvent verifies the end-to-end controller + handler pipeline
// for a pod with container issues. The alert manager has no providers, which
// tests fault tolerance at the delivery boundary. Asserts the controller does
// not panic and shuts down cleanly.
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

	alertMgr := &alert.AlertManager{}
	alertMgr.Init(map[string]map[string]interface{}{}, &config.App{
		DisableStartupMessage: true,
	})

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

	err := h.ProcessPodObject(pod, false)
	if err != nil {
		t.Fatalf("ProcessPodObject failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	ctrlCancel()
	wg.Wait()
	if runErr != nil {
		t.Errorf("controller.Run returned error: %v", runErr)
	}
}
