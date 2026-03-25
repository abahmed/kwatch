package watcher

import (
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	toolsWatch "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/workqueue"
)

type watcherEvent struct {
	eventType string
	obj       runtime.Object
}

type Watcher struct {
	name        string
	watcher     *toolsWatch.RetryWatcher
	queue       *workqueue.Type
	handlerFunc func(string, runtime.Object)
}

// run starts the watcher
func (w *Watcher) run(stopCh chan struct{}) {
	defer utilruntime.HandleCrash()
	defer w.queue.ShutDown()

	logrus.Infof("starting %s watcher", w.name)

	go wait.Until(w.processEvents, time.Second, stopCh)
	go wait.Until(w.runWorker, time.Second, stopCh)

	<-stopCh
}

func (w *Watcher) processEvents() {
	if w.watcher == nil {
		return
	}

	for event := range w.watcher.ResultChan() {
		w.queue.Add(watcherEvent{
			eventType: string(event.Type),
			obj:       event.Object.DeepCopyObject(),
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

	w.handlerFunc(ev.eventType, ev.obj)

	return true
}
