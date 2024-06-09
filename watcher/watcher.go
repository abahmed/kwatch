package watcher

import (
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	toolsWatch "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/workqueue"
)

type watcherEvent struct {
	eventType string
	pod       *corev1.Pod
}

type Watcher struct {
	watcher     *toolsWatch.RetryWatcher
	queue       *workqueue.Type
	handlerFunc func(string, *corev1.Pod)
}

// run starts the watcher
func (w *Watcher) run(stopCh chan struct{}) {
	defer utilruntime.HandleCrash()
	defer w.queue.ShutDown()

	logrus.Info("starting pod watcher")

	go wait.Until(w.processEvents, time.Second, stopCh)
	go wait.Until(w.runWorker, time.Second, stopCh)

	<-stopCh
}

func (w *Watcher) processEvents() {
	if w.watcher == nil {
		return
	}

	for event := range w.watcher.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			logrus.Warnf("failed to cast event to pod object: %v", event.Object)
			continue
		}

		w.queue.Add(watcherEvent{
			eventType: string(event.Type),
			pod:       pod.DeepCopy(),
		})
	}
}

func (w *Watcher) runWorker() {
	for w.processNextItem() {
		// continue looping
	}
}

func (w *Watcher) processNextItem() bool {
	newEvent, quit := w.queue.Get()
	if quit {
		return false
	}

	defer w.queue.Done(newEvent)

	ev, ok := newEvent.(watcherEvent)
	if !ok {
		logrus.Errorf("failed to cast watcher event: %v", ev)
		return true
	}

	w.handlerFunc(ev.eventType, ev.pod)

	return true
}
