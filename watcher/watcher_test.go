package watcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/workqueue"
)

type MockWatcher struct {
	ch     chan watch.Event
	closed bool
}

func NewMockWatcher() *MockWatcher {
	return &MockWatcher{
		ch: make(chan watch.Event, 10),
	}
}

func (m *MockWatcher) ResultChan() <-chan watch.Event {
	return m.ch
}

func (m *MockWatcher) Stop() {
	if !m.closed {
		m.closed = true
		close(m.ch)
	}
}

func (m *MockWatcher) Send(event watch.Event) {
	if !m.closed {
		m.ch <- event
	}
}

type MockWatcherFactory struct {
	watcher *MockWatcher
}

func NewMockWatcherFactory(w *MockWatcher) *MockWatcherFactory {
	return &MockWatcherFactory{watcher: w}
}

func (f *MockWatcherFactory) CreateWatcher(_ interface{}, _ interface{}) (WatchInterface, error) {
	return f.watcher, nil
}

func TestWatcherEvent(t *testing.T) {
	assert := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	ev := watcherEvent{
		eventType: "ADDED",
		obj:       pod,
	}

	assert.Equal("ADDED", ev.eventType)
	assert.NotNil(ev.obj)
}

func TestProcessNextItemQuit(t *testing.T) {
	assert := assert.New(t)

	called := false
	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			called = true
		},
	}

	w.queue.ShutDown()

	result := w.processNextItem()
	assert.False(result)
	assert.False(called)
}

func TestProcessNextItem(t *testing.T) {
	assert := assert.New(t)

	var receivedType string
	var receivedObj runtime.Object

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			receivedType = evType
			receivedObj = obj
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	w.queue.Add(watcherEvent{
		eventType: "ADDED",
		obj:       pod,
	})

	result := w.processNextItem()
	assert.True(result)
	assert.Equal("ADDED", receivedType)
	assert.NotNil(receivedObj)
}

func TestProcessNextItemMultipleEvents(t *testing.T) {
	assert := assert.New(t)

	var eventTypes []string

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			eventTypes = append(eventTypes, evType)
		},
	}

	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}
	pod2 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}}

	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod1})
	w.queue.Add(watcherEvent{eventType: "MODIFIED", obj: pod2})

	result1 := w.processNextItem()
	assert.True(result1)

	result2 := w.processNextItem()
	assert.True(result2)

	assert.Equal([]string{"ADDED", "MODIFIED"}, eventTypes)
}

func TestProcessEventsNilWatcher(t *testing.T) {
	assert := assert.New(t)

	w := &Watcher{
		name:        "test",
		watcher:     nil,
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	w.processEvents()

	assert.True(w.queue.Len() == 0)
}

func TestRunWorker(t *testing.T) {
	assert := assert.New(t)

	var handled bool

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			handled = true
		},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}
	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})

	go w.runWorker()

	time.Sleep(100 * time.Millisecond)
	w.queue.ShutDown()

	time.Sleep(100 * time.Millisecond)
	assert.True(handled)
}

func TestWatcherWithDifferentEventTypes(t *testing.T) {
	assert := assert.New(t)

	var receivedEvents []struct {
		EventType string
	}

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			receivedEvents = append(receivedEvents, struct{ EventType string }{EventType: evType})
		},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}

	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})
	w.queue.Add(watcherEvent{eventType: "MODIFIED", obj: pod})
	w.queue.Add(watcherEvent{eventType: "DELETED", obj: pod})

	for i := 0; i < 3; i++ {
		w.processNextItem()
	}

	assert.Equal("ADDED", receivedEvents[0].EventType)
	assert.Equal("MODIFIED", receivedEvents[1].EventType)
	assert.Equal("DELETED", receivedEvents[2].EventType)
}

func TestWatcherQueueOperations(t *testing.T) {
	assert := assert.New(t)

	queue := workqueue.NewTyped[watcherEvent]()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	event := watcherEvent{eventType: "ADDED", obj: pod}

	queue.Add(event)
	assert.Equal(1, queue.Len())

	item, quit := queue.Get()
	assert.False(quit)
	assert.Equal("ADDED", item.eventType)

	queue.Done(item)
	assert.Equal(0, queue.Len())

	queue.ShutDown()

	_, quit = queue.Get()
	assert.True(quit)
}

func TestWatcherEventTypeString(t *testing.T) {
	assert := assert.New(t)

	types := []string{"ADDED", "MODIFIED", "DELETED", "BOOKMARK", "ERROR"}
	for _, et := range types {
		ev := watcherEvent{eventType: et}
		assert.Equal(et, ev.eventType)
	}
}

func TestWatcherStruct(t *testing.T) {
	assert := assert.New(t)

	w := &Watcher{
		name:        "test-watcher",
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	assert.Equal("test-watcher", w.name)
	assert.NotNil(w.queue)
	assert.NotNil(w.handlerFunc)
}

func TestProcessNextItemProcessesCorrectEvent(t *testing.T) {
	assert := assert.New(t)

	var processedEventType string
	var processedObj runtime.Object

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			processedEventType = evType
			processedObj = obj
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	w.queue.Add(watcherEvent{
		eventType: "DELETED",
		obj:       node,
	})

	result := w.processNextItem()
	assert.True(result)
	assert.Equal("DELETED", processedEventType)
	assert.NotNil(processedObj)
}

func TestRunWorkerProcessesMultipleItems(t *testing.T) {
	assert := assert.New(t)

	var processedEvents []string

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			processedEvents = append(processedEvents, evType)
		},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})
	w.queue.Add(watcherEvent{eventType: "MODIFIED", obj: pod})
	w.queue.Add(watcherEvent{eventType: "DELETED", obj: pod})

	go w.runWorker()

	time.Sleep(200 * time.Millisecond)
	w.queue.ShutDown()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(3, len(processedEvents))
}

func TestProcessNextItemEmptyQueue(t *testing.T) {
	assert := assert.New(t)

	called := false
	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			called = true
		},
	}

	w.queue.ShutDown()
	result := w.processNextItem()
	assert.False(result)
	assert.False(called)
}

func TestWatcherEventWithNilObject(t *testing.T) {
	assert := assert.New(t)

	ev := watcherEvent{
		eventType: "DELETED",
		obj:       nil,
	}

	assert.Equal("DELETED", ev.eventType)
	assert.Nil(ev.obj)
}

func TestWatcherEventWithDifferentKinds(t *testing.T) {
	assert := assert.New(t)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}

	ev1 := watcherEvent{eventType: "ADDED", obj: pod}
	ev2 := watcherEvent{eventType: "MODIFIED", obj: node}

	assert.NotNil(ev1.obj)
	assert.NotNil(ev2.obj)
}

func TestProcessNextItemMarksDone(t *testing.T) {
	assert := assert.New(t)

	w := &Watcher{
		name:        "test",
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})

	assert.Equal(1, w.queue.Len())

	w.processNextItem()
	assert.Equal(0, w.queue.Len())
}

func TestRunWorkerEmptyThenAdd(t *testing.T) {
	assert := assert.New(t)

	var processedEvents []string

	w := &Watcher{
		name:  "test",
		queue: workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			processedEvents = append(processedEvents, evType)
		},
	}

	go w.runWorker()

	time.Sleep(50 * time.Millisecond)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})

	time.Sleep(100 * time.Millisecond)
	w.queue.ShutDown()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(1, len(processedEvents))
}

func TestWatcherQueueWithDifferentObjects(t *testing.T) {
	assert := assert.New(t)

	w := &Watcher{
		name:        "test",
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}

	w.queue.Add(watcherEvent{eventType: "ADDED", obj: pod})
	w.queue.Add(watcherEvent{eventType: "ADDED", obj: node})

	assert.Equal(2, w.queue.Len())

	w.processNextItem()
	assert.Equal(1, w.queue.Len())

	w.processNextItem()
	assert.Equal(0, w.queue.Len())
}

func TestMockWatcher(t *testing.T) {
	assert := assert.New(t)

	mw := NewMockWatcher()
	assert.NotNil(mw)
	assert.NotNil(mw.ResultChan())

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	mw.Send(watch.Event{Type: watch.Added, Object: pod})

	select {
	case ev := <-mw.ResultChan():
		assert.Equal(watch.Added, ev.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	mw.Stop()
	assert.True(mw.closed)
}

func TestProcessEventsWithMockWatcher(t *testing.T) {
	assert := assert.New(t)

	mw := NewMockWatcher()

	w := &Watcher{
		name:        "test",
		watcher:     mw,
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	mw.Send(watch.Event{Type: watch.Added, Object: pod})
	mw.Send(watch.Event{Type: watch.Modified, Object: pod})

	go w.processEvents()

	time.Sleep(100 * time.Millisecond)

	assert.Equal(2, w.queue.Len())

	mw.Stop()
	time.Sleep(100 * time.Millisecond)
}

func TestRunWithMockWatcher(t *testing.T) {
	assert := assert.New(t)

	var receivedEvents []string
	mw := NewMockWatcher()

	w := &Watcher{
		name:    "test",
		watcher: mw,
		queue:   workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			receivedEvents = append(receivedEvents, evType)
		},
	}

	stopCh := make(chan struct{})
	go w.run(stopCh)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	mw.Send(watch.Event{Type: watch.Added, Object: pod})

	time.Sleep(200 * time.Millisecond)
	mw.Stop()

	time.Sleep(200 * time.Millisecond)
	close(stopCh)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(1, len(receivedEvents))
}

func TestRunWorkerProcessesEvents(t *testing.T) {
	assert := assert.New(t)

	var processedEvents []string
	mw := NewMockWatcher()

	w := &Watcher{
		name:    "test",
		watcher: mw,
		queue:   workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {
			processedEvents = append(processedEvents, evType)
		},
	}

	stopCh := make(chan struct{})
	go w.run(stopCh)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	mw.Send(watch.Event{Type: watch.Added, Object: pod})
	mw.Send(watch.Event{Type: watch.Modified, Object: pod})
	mw.Send(watch.Event{Type: watch.Deleted, Object: pod})

	time.Sleep(300 * time.Millisecond)
	mw.Stop()

	time.Sleep(200 * time.Millisecond)
	close(stopCh)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(3, len(processedEvents))
	assert.Equal("ADDED", processedEvents[0])
	assert.Equal("MODIFIED", processedEvents[1])
	assert.Equal("DELETED", processedEvents[2])
}

func TestProcessEventsMultipleCalls(t *testing.T) {
	assert := assert.New(t)

	mw := NewMockWatcher()

	w := &Watcher{
		name:        "test",
		watcher:     mw,
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: func(evType string, obj runtime.Object) {},
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}

	for i := 0; i < 5; i++ {
		mw.Send(watch.Event{Type: watch.Added, Object: pod})
	}

	go w.processEvents()

	time.Sleep(200 * time.Millisecond)
	mw.Stop()

	time.Sleep(100 * time.Millisecond)

	assert.Equal(5, w.queue.Len())
}

func TestWatcherStopClosesChannel(t *testing.T) {
	assert := assert.New(t)

	mw := NewMockWatcher()
	assert.False(mw.closed)

	mw.Stop()
	assert.True(mw.closed)

	mw.Stop()
	assert.True(mw.closed)
}

func TestMockWatcherSendAfterStop(t *testing.T) {
	assert := assert.New(t)

	mw := NewMockWatcher()
	mw.Stop()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	mw.Send(watch.Event{Type: watch.Added, Object: pod})

	assert.True(mw.closed)
}
