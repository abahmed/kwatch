package watcher

import (
	"context"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/handler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/workqueue"
)

type WatcherFactory interface {
	CreateWatcher(ctx context.Context, watchFunc func(options metav1.ListOptions) (watch.Interface, error)) (WatchInterface, error)
}

type RetryWatcherFactory struct{}

func (f *RetryWatcherFactory) CreateWatcher(ctx context.Context, watchFunc func(options metav1.ListOptions) (watch.Interface, error)) (WatchInterface, error) {
	return toolsWatch.NewRetryWatcherWithContext(
		ctx,
		"1",
		&cache.ListWatch{WatchFunc: watchFunc},
	)
}

// Start creates an instance of watcher after initialization and runs it
func Start(
	ctx context.Context,
	client kubernetes.Interface,
	config *config.Config,
	handler handler.Handler) {

	factory := &RetryWatcherFactory{}
	watchers := []*Watcher{
		newPodWatcher(ctx, client, config, handler.ProcessPod, factory),
	}

	if config.NodeMonitor.Enabled {
		watchers = append(watchers, newNodeWatcher(ctx, client, handler.ProcessNode, factory))
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	for idx := range watchers {
		go watchers[idx].run(stopCh)
	}

	<-stopCh
}

// newNodeWatcher creates watcher for nodes
func newNodeWatcher(
	ctx context.Context,
	client kubernetes.Interface,
	handler func(evType string, obj runtime.Object),
	factory WatcherFactory,
) *Watcher {
	watchFunc :=
		func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Nodes().Watch(
				ctx,
				metav1.ListOptions{},
			)
		}

	return newWatcher(
		ctx,
		"node",
		watchFunc,
		handler,
		factory,
	)
}

// newPodWatcher creates watcher for pods
func newPodWatcher(
	ctx context.Context,
	client kubernetes.Interface,
	config *config.Config,
	handler func(evType string, obj runtime.Object),
	factory WatcherFactory,
) *Watcher {
	namespace := metav1.NamespaceAll
	if len(config.AllowedNamespaces) == 1 {
		namespace = config.AllowedNamespaces[0]
	}

	watchFunc :=
		func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(namespace).Watch(
				ctx,
				metav1.ListOptions{},
			)
		}

	return newWatcher(
		ctx,
		"pod",
		watchFunc,
		handler,
		factory,
	)
}

// newWatcher creates watcher with provided name, watch, and handle functions
func newWatcher(
	ctx context.Context,
	name string,
	watchFunc func(options metav1.ListOptions) (watch.Interface, error),
	handleFunc func(string, runtime.Object),
	factory WatcherFactory,
) *Watcher {
	w, _ := factory.CreateWatcher(ctx, watchFunc)

	return &Watcher{
		name:        name,
		watcher:     w,
		queue:       workqueue.NewTyped[watcherEvent](),
		handlerFunc: handleFunc,
	}
}
