package controller

import (
	"errors"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/provider"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller holds necessary
type Controller struct {
	name      string
	informer  cache.Controller
	indexer   cache.Indexer
	kclient   kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	providers []provider.Provider
}

// run starts the controller
func (c *Controller) run(workers int, stopCh chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	logrus.Infof("starting %s controller", c.name)

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(
			errors.New("timed out waiting for caches to sync"))
		return
	}

	logrus.Infof("%s controller synced and ready", c.name)

	// send notification to providers
	util.SendProvidersMsg(c.providers, fmt.Sprintf(constant.WelcomeMsg, constant.Version))

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
		logrus.Errorf(
			"failed to fetch object %s from store: %s",
			key,
			err.Error())
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Pod, so that we will see
		// a delete for one pod
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

	for _, container := range pod.Status.ContainerStatuses {
		// filter running containers
		if container.Ready ||
			(container.State.Waiting == nil &&
				container.State.Terminated == nil) {
			continue
		}

		if (container.State.Waiting != nil &&
			container.State.Waiting.Reason == "ContainerCreating") ||
			(container.State.Terminated != nil &&
				container.State.Terminated.Reason == "Completed") {
			continue
		}

		logrus.Debugf(
			"processing container %s in pod %s@%s",
			container.Name,
			pod.Name,
			pod.Namespace)

		// get logs for this container
		logs := util.GetPodContainerLogs(
			c.kclient,
			pod.Name,
			container.Name,
			pod.Namespace,
			container.RestartCount > 0)

		// get events for this pod
		eventsString :=
			util.GetPodEventsStr(c.kclient, pod.Name, pod.Namespace)

		// get reason according to state
		reason := "Unknown"
		if container.State.Waiting != nil {
			reason = container.State.Waiting.Reason
		} else if container.State.Terminated != nil {
			reason = container.State.Terminated.Reason
		}

		evnt := event.Event{
			Name:      pod.Name,
			Container: container.Name,
			Namespace: pod.Namespace,
			Reason:    reason,
			Logs:      logs,
			Events:    eventsString,
		}

		// send event to providers
		util.SendProvidersEvent(c.providers, evnt)
	}

	return nil
}
