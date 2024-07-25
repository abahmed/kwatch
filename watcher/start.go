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

// Start creates an instance of watcher after initialization and runs it
func Start(
	client kubernetes.Interface,
	config *config.Config,
	handler handler.Handler) {

	watchers := []*Watcher{
		newPodWatcher(client, config, handler.ProcessPod),
	}

	if config.NodeMonitor.Enabled {
		watchers = append(watchers, newNodeWatcher(client, handler.ProcessNode))
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
	client kubernetes.Interface,
	handler func(evType string, obj runtime.Object),
) *Watcher {
	watchFunc :=
		func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Nodes().Watch(
				context.Background(),
				metav1.ListOptions{},
			)
		}

	return newWatcher(
		"node",
		watchFunc,
		handler,
	)
}

// newPodWatcher creates watcher for pods
func newPodWatcher(
	client kubernetes.Interface,
	config *config.Config,
	handler func(evType string, obj runtime.Object),
) *Watcher {
	namespace := metav1.NamespaceAll
	if len(config.AllowedNamespaces) == 1 {
		namespace = config.AllowedNamespaces[0]
	}

	watchFunc :=
		func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(namespace).Watch(
				context.Background(),
				metav1.ListOptions{},
			)
		}

	return newWatcher(
		"pod",
		watchFunc,
		handler,
	)
}

// newWatcher creates watcher with provided name, watch, and handle functions
func newWatcher(
	name string,
	watchFunc func(options metav1.ListOptions) (watch.Interface, error),
	handleFunc func(string, runtime.Object),
) *Watcher {
	watcher, _ :=
		toolsWatch.NewRetryWatcher(
			"1",
			&cache.ListWatch{WatchFunc: watchFunc},
		)

	return &Watcher{
		name:        name,
		watcher:     watcher,
		queue:       workqueue.New(),
		handlerFunc: handleFunc,
	}
}
