package controller

import (
	"errors"
	"time"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/storage"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/strings/slices"
)

// Controller holds necessary
type Controller struct {
	name         string
	informer     cache.Controller
	indexer      cache.Indexer
	kclient      kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	alertManager *alertmanager.AlertManager
	store        storage.Storage
	config       *config.Config
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
		// Below we will warm up our cache with a Pod, so that we will see a
		// delete for one pod
		logrus.Infof("pod %s does not exist anymore\n", key)

		c.store.DelPod(key)

		// Clean up intervals if possible
		return nil
	}

	pod, ok := obj.(*v1.Pod)
	if !ok {
		logrus.Warnf("failed to cast to pod object: %v", obj)

		// to avoid re-queuing it
		return nil
	}

	// filter by namespaces in config if specified
	if len(c.config.AllowedNamespaces) > 0 &&
		!slices.Contains(c.config.AllowedNamespaces, pod.Namespace) {
		logrus.Infof(
			"skip namespace %s as not in namespace allow list",
			pod.Namespace)
		return nil
	}
	if len(c.config.ForbiddenNamespaces) > 0 &&
		slices.Contains(c.config.ForbiddenNamespaces, pod.Namespace) {
		logrus.Infof(
			"skip namespace %s as in namespace forbid list",
			pod.Namespace)
		return nil
	}

	c.processPod(key, pod)

	return nil
}

// processPod checks status of pod and notify in abnormal cases
func (c *Controller) processPod(key string, pod *v1.Pod) {
	for _, container := range pod.Status.ContainerStatuses {
		// filter running containers
		if container.Ready ||
			(container.State.Waiting == nil &&
				container.State.Terminated == nil) {
			c.store.DelPodContainer(key, container.Name)
			continue
		}

		if container.State.Waiting != nil {
			switch {
			case container.State.Waiting.Reason == "ContainerCreating":
				continue
			case container.State.Waiting.Reason == "PodInitializing":
				continue
			}
		} else if container.State.Terminated != nil {
			switch {
			case container.State.Terminated.Reason == "Completed":
				continue
			case container.State.Terminated.ExitCode == 143:
				// 143 is the exit code for graceful termination
				continue
			case container.State.Terminated.ExitCode == 0:
				// 0 is the exit code for purpose stop
				continue
			}
		}

		// if reported, continue
		if c.store.HasPodContainer(key, container.Name) {
			continue
		}

		if c.config.IgnoreFailedGracefulShutdown &&
			util.ContainsKillingStoppingContainerEvents(
				c.kclient,
				pod.Name,
				pod.Namespace) {
			// Graceful shutdown did not work and container was killed during
			// shutdown.
			//  Not really an error
			continue
		}

		logrus.Debugf(
			"processing container %s in pod %s@%s",
			container.Name,
			pod.Name,
			pod.Namespace)

		// get reason according to state
		reason := "Unknown"
		if container.State.Waiting != nil {
			reason = container.State.Waiting.Reason
		} else if container.State.Terminated != nil {
			reason = container.State.Terminated.Reason
		}

		if len(c.config.AllowedReasons) > 0 &&
			!slices.Contains(c.config.AllowedReasons, reason) {
			logrus.Infof("skip reason %s as not in reason allow list", reason)
			return
		}
		if len(c.config.ForbiddenReasons) > 0 &&
			slices.Contains(c.config.ForbiddenReasons, reason) {
			logrus.Infof("skip reason %s as in reason forbid list", reason)
			return
		}

		if len(c.config.IgnoreContainerNames) > 0 &&
			slices.Contains(c.config.IgnoreContainerNames, container.Name) {
			logrus.Infof(
				"skip pod %s as in container ignore list",
				container.Name)
			return
		}

		// get logs for this container
		previous := true
		if reason == "Error" {
			previous = false
		} else if container.RestartCount > 0 {
			previous = true
		}

		logs := util.GetPodContainerLogs(
			c.kclient,
			pod.Name,
			container.Name,
			pod.Namespace,
			previous,
			c.config.MaxRecentLogLines)

		// get events for this pod
		eventsString :=
			util.GetPodEventsStr(c.kclient, pod.Name, pod.Namespace)

		evnt := event.Event{
			Name:      pod.Name,
			Container: container.Name,
			Namespace: pod.Namespace,
			Reason:    reason,
			Logs:      logs,
			Events:    eventsString,
			Labels:    pod.Labels,
		}

		// save container as it's reported to avoid duplication
		c.store.AddPodContainer(key, container.Name)

		// send event to providers
		c.alertManager.NotifyEvent(evnt)
	}
}
