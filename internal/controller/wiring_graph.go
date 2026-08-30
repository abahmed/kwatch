package controller

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

// wireConfigMap wires the shared configmap informer used for dependency
// tracking. It records changes on every monitored namespace.
func (c *Controller) wireConfigMap(fs factorySet) {
	cmLister := fs.configMapLister()
	cmInformers := fs.configMapInformers()

	c.configMapLister = cmLister
	for _, inf := range cmInformers {
		c.configMapSynced = append(c.configMapSynced, inf.HasSynced)
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeCreate, "configmap", obj)
			},
			UpdateFunc: func(old, new interface{}) {
				c.recordChange(kwcontext.ChangeUpdate, "configmap", new)
			},
			DeleteFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeDelete, "configmap", obj)
				if c.graph != nil {
					if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
						if ns, name, splitErr := cache.SplitMetaNamespaceKey(key); splitErr == nil {
							c.graph.RemoveNode("configmap", ns, name)
						}
					}
				}
			},
		})
	}
}

// wireGraphSupport assigns the lister extras used by the resource graph.
func (c *Controller) wireGraphSupport(fs factorySet) {
	c.secretLister = fs.secretLister()
	c.pvcLister = fs.pvcLister()
	c.pvLister = fs.persistentVolumeLister()
	c.serviceAccountLister = fs.serviceAccountLister()
	c.storageClassLister = fs.storageClassLister()
	for _, inf := range fs.pvcInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.serviceAccountInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.persistentVolumeInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.storageClassInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.secretInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeCreate, "secret", obj)
			},
			UpdateFunc: func(_, obj interface{}) {
				c.recordChange(kwcontext.ChangeUpdate, "secret", obj)
			},
			DeleteFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeDelete, "secret", obj)
				if c.graph != nil {
					if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
						if ns, name, splitErr := cache.SplitMetaNamespaceKey(key); splitErr == nil {
							c.graph.RemoveNode("secret", ns, name)
						}
					}
				}
			},
		})
	}
	// Cluster-scoped listers only exist when a global/cluster factory was
	// created; watching multiple namespaces skips PV and storage class edges.
	if c.pvLister == nil || c.storageClassLister == nil {
		klog.InfoS(
			"multi-namespace watch: persistentvolume and storageclass graph " +
				"edges are unavailable",
		)
	}
}
