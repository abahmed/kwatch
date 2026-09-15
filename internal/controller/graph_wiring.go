package controller

import "github.com/abahmed/kwatch/internal/config"

// wireGraphHandlers attaches graph maintenance to the shared informers.
func (c *Controller) wireGraphHandlers(
	fs factorySet,
	runtime config.RuntimeConfig,
) {
	c.wireNodeAndLeaseGraphHandlers(fs)
	if c.serviceLister != nil {
		for _, inf := range fs.serviceInformers() {
			inf.AddEventHandler(c.graphHandler("service", c.rebuildService))
		}
	}
	for _, inf := range fs.rsInformers() {
		inf.AddEventHandler(c.graphHandler(
			"replicaset", func(obj interface{}) { c.rebuildReplicaSet(obj) },
		))
	}
	for _, inf := range fs.pvcInformers() {
		inf.AddEventHandler(c.graphHandler("pvc", c.rebuildPersistentVolumeClaim))
	}
	for _, inf := range fs.persistentVolumeInformers() {
		inf.AddEventHandler(c.graphHandler(
			"persistentvolume", c.rebuildPersistentVolume,
		))
	}
	for _, inf := range fs.storageClassInformers() {
		inf.AddEventHandler(c.graphHandler("storageclass", func(interface{}) {}))
	}

	if runtime.JobMonitor().Enabled {
		for _, inf := range fs.jobInformers() {
			inf.AddEventHandler(c.graphHandler("job", c.rebuildJob))
		}
	}
	if runtime.IngressMonitor().Enabled {
		for _, inf := range fs.ingressInformers() {
			inf.AddEventHandler(c.graphHandler("ingress", c.rebuildIngress))
		}
	}
	if runtime.HpaMonitor().Enabled {
		for _, inf := range fs.hpaInformers() {
			inf.AddEventHandler(c.graphHandler(
				"horizontalpodautoscaler",
				c.rebuildHorizontalPodAutoscaler,
			))
		}
	}
	if runtime.NetworkPolicyMonitor().Enabled {
		for _, inf := range fs.netpolInformers() {
			inf.AddEventHandler(c.graphHandler(
				"networkpolicy", c.rebuildNetworkPolicy,
			))
		}
	}
	if runtime.PdbMonitor().Enabled {
		for _, inf := range fs.pdbInformers() {
			inf.AddEventHandler(c.graphHandler(
				"poddisruptionbudget", c.rebuildPodDisruptionBudget,
			))
		}
	}
	if c.endpointSliceLister != nil {
		for _, inf := range fs.endpointSliceInformers() {
			inf.AddEventHandler(c.graphHandler(
				"endpointslice", c.rebuildEndpointSlice,
			))
		}
	}
}

func (c *Controller) wireNodeAndLeaseGraphHandlers(fs factorySet) {
	if c.nodeLister != nil {
		inf := fs.nodeInformer()
		inf.AddEventHandler(c.graphHandler("node", c.rebuildNodeGraph))
	}
	if c.leaseLister != nil {
		for _, inf := range fs.leaseInformers() {
			inf.AddEventHandler(c.graphHandler("lease", c.rebuildLeaseGraph))
		}
	}
}
