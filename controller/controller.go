package controller

import (
	"context"
	"errors"
	"time"

	"github.com/abahmed/kwatch/client"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/provider"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	name            string
	informer        cache.Controller
	indexer         cache.Indexer
	kclient         kubernetes.Interface
	queue           workqueue.RateLimitingInterface
	serverStartTime time.Time
	providers       []provider.Provider
}

func Start() {
	// create kubernetes client
	kclient := client.Create()

	// create rate limiting queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "status.phase!=Running"
				return kclient.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "status.phase!=Running"
				return kclient.CoreV1().Pods(v1.NamespaceAll).Watch(context.TODO(), options)
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
				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					logrus.Debugf("received delete for Pod %s\n", key)
					queue.Add(key)
				}
			},
		}, cache.Indexers{})

	con := Controller{
		name:      "pod-crash",
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		kclient:   kclient,
		providers: []provider.Provider{provider.NewSlack()},
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	con.Run(constant.NumWorkers, stopCh)
}

func (c *Controller) Run(workers int, stopCh chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.serverStartTime = time.Now().Local()
	logrus.Infof("starting %s controller", c.name)

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(errors.New("timed out waiting for caches to sync"))
		return
	}

	logrus.Infof("%s controller synced and ready", c.name)

	// send notification to providers
	for _, prv := range c.providers {
		if err := prv.SendMessage(constant.WelcomeMsg); err != nil {
			logrus.Errorf("failed to send msg with %s: %s", prv.Name(), err.Error())
		}
	}

	// start workers
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()

	if quit {
		return false
	}

	defer c.queue.Done(newEvent)

	err := c.processItem(newEvent.(string))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < constant.NumRequeues {
		logrus.Errorf("failed to process %v (will retry): %v", newEvent, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		logrus.Errorf("failed to process %v (giving up): %v", newEvent, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		logrus.Errorf("failed to fetch object %s from store: %s", key, err.Error())
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Pod, so that we will see a delete for one pod
		logrus.Infof("pod %s does not exist anymore\n", key)

		// Clean up intervals if possible
		return nil
	}

	pod, ok := obj.(*v1.Pod)
	if !ok {
		logrus.Warnf("failed to cast to pod object: %v", obj)

		// to avoid re-queuing it
		return nil
	}

	// ignore messages happened before starting controller
	if pod.CreationTimestamp.Sub(c.serverStartTime).Seconds() <= 0 {
		return nil
	}

	for _, container := range pod.Status.ContainerStatuses {
		// filter running containers
		if container.Ready || container.State.Waiting == nil {
			continue
		}

		switch container.State.Waiting.Reason {
		case "CrashLoopBackOff":
		case "ImagePullBackOff":
		case "ErrImagePull":
			// retrieve logs of container
			logs := util.GetPodContainerLogs(c.kclient, pod.Name, container.Name, pod.Namespace)

			// get only failed events
			eventsString := util.GetPodFailedEvents(c.kclient, pod.Name, pod.Namespace)

			evnt := event.Event{
				Name:      pod.Name,
				Container: container.Name,
				Namespace: pod.Namespace,
				Reason:    container.State.Waiting.Reason,
				Logs:      logs,
				Events:    eventsString,
			}

			// send notification to providers
			for _, prv := range c.providers {
				if err := prv.SendEvent(&evnt); err != nil {
					logrus.Errorf("failed to send event with %s: %s", prv.Name(), err.Error())
				}
			}
		}
	}

	return nil
}
