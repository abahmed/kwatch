package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) runPodWorker(ctx context.Context) {
	for c.processNextPodItem(ctx) {
	}
}

func (c *Controller) runNodeWorker(ctx context.Context) {
	for c.processNextNodeItem() {
	}
}

func (c *Controller) runDeploymentWorker(ctx context.Context) {
	for c.processNextDeploymentItem() {
	}
}

func (c *Controller) runJobWorker(ctx context.Context) {
	for c.processNextJobItem() {
	}
}

func (c *Controller) runDaemonSetWorker(ctx context.Context) {
	for c.processNextDaemonSetItem() {
	}
}

func (c *Controller) runStatefulSetWorker(ctx context.Context) {
	for c.processNextStatefulSetItem() {
	}
}

func (c *Controller) runPdbWorker(ctx context.Context) {
	for c.processNextPdbItem() {
	}
}

func (c *Controller) runCronJobWorker(ctx context.Context) {
	for c.processNextCronJobItem() {
	}
}

func (c *Controller) runHorizontalPodAutoscalerWorker(ctx context.Context) {
	for c.processNextHorizontalPodAutoscalerItem() {
	}
}

func (c *Controller) runServiceWorker(ctx context.Context) {
	for c.processNextServiceItem() {
	}
}

func (c *Controller) runEndpointSliceWorker(ctx context.Context) {
	for c.processNextEndpointSliceItem() {
	}
}

func (c *Controller) runMwcWorker(ctx context.Context) {
	for c.processNextMwcItem() {
	}
}

func (c *Controller) runVwcWorker(ctx context.Context) {
	for c.processNextVwcItem() {
	}
}

func (c *Controller) runIngressWorker(ctx context.Context) {
	for c.processNextIngressItem() {
	}
}

func (c *Controller) runNetpolWorker(ctx context.Context) {
	for c.processNextNetpolItem() {
	}
}

func (c *Controller) runCpPodWorker(ctx context.Context) {
	for c.processNextCpPodItem() {
	}
}

func (c *Controller) processNextPodItem(ctx context.Context) bool {
	key, quit := c.podQueue.Get()
	if quit {
		return false
	}
	defer c.podQueue.Done(key)

	if err := c.syncPod(ctx, key); err != nil {
		c.podQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing pod %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.podQueue.Forget(key)
	return true
}

func (c *Controller) processNextNodeItem() bool {
	key, quit := c.nodeQueue.Get()
	if quit {
		return false
	}
	defer c.nodeQueue.Done(key)

	if err := c.syncNode(key); err != nil {
		c.nodeQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing node %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.nodeQueue.Forget(key)
	return true
}

func (c *Controller) processNextDeploymentItem() bool {
	key, quit := c.deploymentQueue.Get()
	if quit {
		return false
	}
	defer c.deploymentQueue.Done(key)

	if err := c.syncDeployment(key); err != nil {
		c.deploymentQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing deployment %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.deploymentQueue.Forget(key)
	return true
}

func (c *Controller) processNextJobItem() bool {
	key, quit := c.jobQueue.Get()
	if quit {
		return false
	}
	defer c.jobQueue.Done(key)

	if err := c.syncJob(key); err != nil {
		c.jobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing job %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.jobQueue.Forget(key)
	return true
}
func (c *Controller) syncPod(ctx context.Context, key string) error {
	return c.handler.ProcessPod(ctx, key, false)
}

func (c *Controller) syncNode(key string) error {
	return c.handler.ProcessNode(key, false)
}

func (c *Controller) syncDeployment(key string) error {
	return c.handler.ProcessDeployment(key, false)
}

func (c *Controller) processNextDaemonSetItem() bool {
	key, quit := c.daemonSetQueue.Get()
	if quit {
		return false
	}
	defer c.daemonSetQueue.Done(key)

	if err := c.syncDaemonSet(key); err != nil {
		c.daemonSetQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing daemonset %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.daemonSetQueue.Forget(key)
	return true
}

func (c *Controller) processNextStatefulSetItem() bool {
	key, quit := c.statefulSetQueue.Get()
	if quit {
		return false
	}
	defer c.statefulSetQueue.Done(key)

	if err := c.syncStatefulSet(key); err != nil {
		c.statefulSetQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing statefulset %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.statefulSetQueue.Forget(key)
	return true
}

func (c *Controller) processNextPdbItem() bool {
	key, quit := c.pdbQueue.Get()
	if quit {
		return false
	}
	defer c.pdbQueue.Done(key)

	if err := c.syncPdb(key); err != nil {
		c.pdbQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing pdb %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.pdbQueue.Forget(key)
	return true
}

func (c *Controller) processNextCronJobItem() bool {
	key, quit := c.cronJobQueue.Get()
	if quit {
		return false
	}
	defer c.cronJobQueue.Done(key)

	if err := c.syncCronJob(key); err != nil {
		c.cronJobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing cronjob %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.cronJobQueue.Forget(key)
	return true
}

func (c *Controller) syncDaemonSet(key string) error {
	return c.handler.ProcessDaemonSet(key, false)
}

func (c *Controller) syncStatefulSet(key string) error {
	return c.handler.ProcessStatefulSet(key, false)
}

func (c *Controller) syncPdb(key string) error {
	return c.handler.ProcessPdb(key, false)
}

func (c *Controller) syncCronJob(key string) error {
	return c.handler.ProcessCronJob(key, false)
}

func (c *Controller) processNextHorizontalPodAutoscalerItem() bool {
	key, quit := c.hpaQueue.Get()
	if quit {
		return false
	}
	defer c.hpaQueue.Done(key)

	if err := c.syncHorizontalPodAutoscaler(key); err != nil {
		c.hpaQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing hpa %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.hpaQueue.Forget(key)
	return true
}

func (c *Controller) syncHorizontalPodAutoscaler(key string) error {
	return c.handler.ProcessHorizontalPodAutoscaler(key, false)
}

func (c *Controller) syncJob(key string) error {
	return c.handler.ProcessJob(key, false)
}
func (c *Controller) syncService(key string) error {
	return c.handler.ProcessService(key, false)
}

func (c *Controller) syncEndpointSlice(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// EndpointSlice names are "<service-name>-<hash>"; resolve the owning
	// Service via the kubernetes.io/service-name label, not the slice name.
	epSlice, err := c.endpointSliceLister.EndpointSlices(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	serviceName := epSlice.Labels["kubernetes.io/service-name"]
	if serviceName == "" {
		return nil
	}

	return c.handler.ProcessService(namespace+"/"+serviceName, false)
}

func (c *Controller) syncMwc(key string) error {
	return c.handler.ProcessMutatingWebhookConfiguration(key, false)
}

func (c *Controller) syncVwc(key string) error {
	return c.handler.ProcessValidatingWebhookConfiguration(key, false)
}

func (c *Controller) syncIngress(key string) error {
	return c.handler.ProcessIngress(key, false)
}

func (c *Controller) syncNetpol(key string) error {
	return c.handler.ProcessNetworkPolicy(key, false)
}

func (c *Controller) syncCpPod(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.cpPodLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.handler.ProcessControlPlanePod(pod)
}

func (c *Controller) processNextServiceItem() bool {
	key, quit := c.serviceQueue.Get()
	if quit {
		return false
	}
	defer c.serviceQueue.Done(key)

	if err := c.syncService(key); err != nil {
		c.serviceQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing service %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.serviceQueue.Forget(key)
	return true
}

func (c *Controller) processNextEndpointSliceItem() bool {
	key, quit := c.endpointSliceQueue.Get()
	if quit {
		return false
	}
	defer c.endpointSliceQueue.Done(key)

	if err := c.syncEndpointSlice(key); err != nil {
		c.endpointSliceQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing endpointslice %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.endpointSliceQueue.Forget(key)
	return true
}

func (c *Controller) processNextMwcItem() bool {
	key, quit := c.mwcQueue.Get()
	if quit {
		return false
	}
	defer c.mwcQueue.Done(key)

	if err := c.syncMwc(key); err != nil {
		c.mwcQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing mutatingwebhookconfiguration %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.mwcQueue.Forget(key)
	return true
}

func (c *Controller) processNextVwcItem() bool {
	key, quit := c.vwcQueue.Get()
	if quit {
		return false
	}
	defer c.vwcQueue.Done(key)

	if err := c.syncVwc(key); err != nil {
		c.vwcQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing validatingwebhookconfiguration %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.vwcQueue.Forget(key)
	return true
}

func (c *Controller) processNextIngressItem() bool {
	key, quit := c.ingressQueue.Get()
	if quit {
		return false
	}
	defer c.ingressQueue.Done(key)

	if err := c.syncIngress(key); err != nil {
		c.ingressQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing ingress %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.ingressQueue.Forget(key)
	return true
}

func (c *Controller) processNextNetpolItem() bool {
	key, quit := c.netpolQueue.Get()
	if quit {
		return false
	}
	defer c.netpolQueue.Done(key)

	if err := c.syncNetpol(key); err != nil {
		c.netpolQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing networkpolicy %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.netpolQueue.Forget(key)
	return true
}

func (c *Controller) processNextCpPodItem() bool {
	key, quit := c.cpPodQueue.Get()
	if quit {
		return false
	}
	defer c.cpPodQueue.Done(key)

	if err := c.syncCpPod(key); err != nil {
		c.cpPodQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing controlplane pod %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.cpPodQueue.Forget(key)
	return true
}
