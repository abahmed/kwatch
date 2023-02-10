package controller

import (
	"context"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	memory "github.com/abahmed/kwatch/storage/memory"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Start creates an instance of controller after initialization and runs it
func Start(
	client kubernetes.Interface,
	alertManager *alertmanager.AlertManager,
	config *config.Config) {
	// create rate limiting queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Namespace to watch, if all is selected it will watch all namespaces
	// in a cluster scope, if not then it will watch only in the namespace
	var namespaceToWatch = v1.NamespaceAll

	// if there is exactly 1 namespace listen only to that namespace for events
	if len(config.AllowedNamespaces) == 1 {
		namespaceToWatch = config.AllowedNamespaces[0]
	}

	indexer, informer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().
					Pods(namespaceToWatch).
					List(context.TODO(), opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().
					Pods(namespaceToWatch).
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
			UpdateFunc: func(_, new interface{}) {
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
		name:         "pod-crash",
		informer:     informer,
		indexer:      indexer,
		queue:        queue,
		kclient:      client,
		alertManager: alertManager,
		store:        memory.NewMemory(),
		config:       config,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	controller.run(constant.NumWorkers, stopCh)
}
