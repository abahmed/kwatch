package controller

import (
	"context"

	"github.com/abahmed/kwatch/client"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/provider"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Start creates an instance of controller after initialization and runs it
func Start() {
	// create kubernetes client
	kclient := client.Create()

	// create rate limiting queue
	queue :=
		workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return kclient.CoreV1().
					Pods(v1.NamespaceAll).
					List(context.TODO(), opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				return kclient.CoreV1().
					Pods(v1.NamespaceAll).
					Watch(context.TODO(), opts)
			},
		},
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					logrus.Debugf("received create for Pod %s\n", key)
					queue.Add(key)
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					logrus.Debugf("received update for Pod %s\n", key)
					queue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// IndexerInformer uses a delta queue, therefore for deletes
				// we have to use this key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					logrus.Debugf("received delete for Pod %s\n", key)
					queue.Add(key)
				}
			},
		}, cache.Indexers{})

	controller := Controller{
		name:      "pod-crash",
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		kclient:   kclient,
		providers: []provider.Provider{provider.NewSlack(), provider.NewDiscord()},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	controller.run(constant.NumWorkers, stopCh)
}
