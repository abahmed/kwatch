package watcher

import (
	"context"
	"log"

	"github.com/abahmed/kwatch/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/workqueue"
)

type Handler interface {
	ProcessPod(evType string, obj runtime.Object)
	ProcessNode(evType string, obj runtime.Object)
}

func Start(
	client kubernetes.Interface,
	cfg *config.Config,
	handler Handler) {

	ctx := context.Background()

	watchers := []*Watcher{
		newPodWatcher(ctx, client, cfg, handler.ProcessPod),
	}

	if cfg.NodeMonitor.Enabled {
		watchers = append(watchers, newNodeWatcher(ctx, client, handler.ProcessNode))
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	for idx := range watchers {
		go watchers[idx].run(stopCh)
	}

	<-stopCh
}

func newNodeWatcher(
	ctx context.Context,
	client kubernetes.Interface,
	handler func(evType string, obj runtime.Object),
) *Watcher {
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		return client.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
	}

	return newWatcher(ctx, "node", watchFunc, handler)
}

func newPodWatcher(
	ctx context.Context,
	client kubernetes.Interface,
	cfg *config.Config,
	handler func(evType string, obj runtime.Object),
) *Watcher {
	namespace := metav1.NamespaceAll
	if len(cfg.AllowedNamespaces) == 1 {
		namespace = cfg.AllowedNamespaces[0]
	}

	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		return client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{})
	}

	return newWatcher(ctx, "pod", watchFunc, handler)
}

func newWatcher(
	ctx context.Context,
	name string,
	watchFunc func(options metav1.ListOptions) (watch.Interface, error),
	handleFunc func(string, runtime.Object),
) *Watcher {
	watcher, err := watchtools.NewRetryWatcherWithContext(ctx, "1", &cache.ListWatch{WatchFunc: watchFunc})
	if err != nil {
		log.Printf("failed to create watcher %s: %v", name, err)
	}

	queue := workqueue.NewTyped[any]()
	return &Watcher{
		name:        name,
		watcher:     watcher,
		handlerFunc: handleFunc,
		queue:       queue,
	}
}
