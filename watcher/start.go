package watcher

import (
	"context"

	"github.com/abahmed/kwatch/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	handleFunc func(string, *corev1.Pod)) {
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

	watcher, _ :=
		toolsWatch.NewRetryWatcher(
			"1",
			&cache.ListWatch{WatchFunc: watchFunc},
		)

	w := &Watcher{
		watcher:     watcher,
		queue:       workqueue.New(),
		handlerFunc: handleFunc,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	w.run(stopCh)
}
