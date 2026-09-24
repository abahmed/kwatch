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
			inf.AddEventHandler(safeEventHandler(
				"service", c.graphHandler("service", c.rebuildService),
			))
		}
	}
	for _, inf := range fs.rsInformers() {
		inf.AddEventHandler(safeEventHandler(
			"replicaset",
			c.graphHandler(
				"replicaset",
				func(obj interface{}) { c.rebuildReplicaSet(obj) },
			),
		))
	}
	for _, inf := range fs.pvcInformers() {
		inf.AddEventHandler(safeEventHandler(
			"pvc", c.graphHandler("pvc", c.rebuildPersistentVolumeClaim),
		))
	}
	for _, inf := range fs.persistentVolumeInformers() {
		inf.AddEventHandler(safeEventHandler(
			"persistentvolume",
			c.graphHandler("persistentvolume", c.rebuildPersistentVolume),
		))
	}
	for _, inf := range fs.storageClassInformers() {
		inf.AddEventHandler(safeEventHandler(
			"storageclass", c.graphHandler("storageclass", func(interface{}) {}),
		))
	}

	if runtime.Monitors().Job().Enabled {
		for _, inf := range fs.jobInformers() {
			inf.AddEventHandler(safeEventHandler(
				"job", c.graphHandler("job", c.rebuildJob),
			))
		}
	}
	if runtime.Monitors().Ingress().Enabled {
		for _, inf := range fs.ingressInformers() {
			inf.AddEventHandler(safeEventHandler(
				"ingress", c.graphHandler("ingress", c.rebuildIngress),
			))
		}
	}
	if runtime.Monitors().HPA().Enabled {
		for _, inf := range fs.hpaInformers() {
			inf.AddEventHandler(safeEventHandler(
				"horizontalpodautoscaler",
				c.graphHandler(
					"horizontalpodautoscaler",
					c.rebuildHorizontalPodAutoscaler,
				),
			))
		}
	}
	if runtime.Monitors().NetworkPolicy().Enabled {
		for _, inf := range fs.netpolInformers() {
			inf.AddEventHandler(safeEventHandler(
				"networkpolicy",
				c.graphHandler("networkpolicy", c.rebuildNetworkPolicy),
			))
		}
	}
	if runtime.Monitors().PDB().Enabled {
		for _, inf := range fs.pdbInformers() {
			inf.AddEventHandler(safeEventHandler(
				"poddisruptionbudget",
				c.graphHandler(
					"poddisruptionbudget", c.rebuildPodDisruptionBudget,
				),
			))
		}
	}
	if c.endpointSliceLister != nil {
		for _, inf := range fs.endpointSliceInformers() {
			inf.AddEventHandler(safeEventHandler(
				"endpointslice",
				c.graphHandler("endpointslice", c.rebuildEndpointSlice),
			))
		}
	}
	if runtime.Monitors().AdmissionWebhook().Enabled {
		fs.mwcInformer().AddEventHandler(safeEventHandler(
			"mutatingwebhookconfiguration",
			c.graphHandler(
				"mutatingwebhookconfiguration",
				c.rebuildMutatingWebhookGraph,
			),
		))
		fs.vwcInformer().AddEventHandler(safeEventHandler(
			"validatingwebhookconfiguration",
			c.graphHandler(
				"validatingwebhookconfiguration",
				c.rebuildValidatingWebhookGraph,
			),
		))
	}
}

func (c *Controller) wireNodeAndLeaseGraphHandlers(fs factorySet) {
	if c.nodeLister != nil {
		inf := fs.nodeInformer()
		inf.AddEventHandler(safeEventHandler(
			"node", c.graphHandler("node", c.rebuildNodeGraph),
		))
	}
	if c.leaseLister != nil {
		for _, inf := range fs.leaseInformers() {
			inf.AddEventHandler(safeEventHandler(
				"lease", c.graphHandler("lease", c.rebuildLeaseGraph),
			))
		}
	}
}
