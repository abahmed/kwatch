package storagegraph

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

var (
	volumeAttachmentGVR = schema.GroupVersionResource{
		Group: "storage.k8s.io", Version: "v1", Resource: "volumeattachments",
	}
	csiDriverGVR = schema.GroupVersionResource{
		Group: "storage.k8s.io", Version: "v1", Resource: "csidrivers",
	}
	volumeSnapshotGVR = schema.GroupVersionResource{
		Group: "snapshot.storage.k8s.io", Version: "v1",
		Resource: "volumesnapshots",
	}
	snapshotContentGVR = schema.GroupVersionResource{
		Group: "snapshot.storage.k8s.io", Version: "v1",
		Resource: "volumesnapshotcontents",
	}
	snapshotClassGVR = schema.GroupVersionResource{
		Group: "snapshot.storage.k8s.io", Version: "v1",
		Resource: "volumesnapshotclasses",
	}
)

func (m *Monitor) resourceSpecs() []dynamicwatch.ResourceSpec {
	specs := make([]dynamicwatch.ResourceSpec, 0, 5)
	for _, watched := range []struct {
		gvr        schema.GroupVersionResource
		fn         func(interface{})
		namespaced bool
	}{
		{volumeAttachmentGVR, m.processVolumeAttachment, false},
		{csiDriverGVR, m.processCSIDriver, false},
		{volumeSnapshotGVR, m.processVolumeSnapshot, true},
		{snapshotContentGVR, m.processSnapshotContent, false},
		{snapshotClassGVR, m.processSnapshotClass, false},
	} {
		fn := watched.fn
		gvr := watched.gvr
		specs = append(specs, dynamicwatch.ResourceSpec{
			GVR: gvr, Namespaced: watched.namespaced,
			Handlers: cache.ResourceEventHandlerFuncs{
				AddFunc:    fn,
				UpdateFunc: func(_, obj interface{}) { fn(obj) },
				DeleteFunc: func(obj interface{}) { m.removeNode(gvr, obj) },
			},
		})
	}
	return specs
}

func (m *Monitor) watchNamespaces(namespaced bool) []string {
	if !namespaced || m.watchAll {
		return []string{""}
	}
	return dynamicwatch.Namespaces(namespaced, m.namespaces, m.watchAll)
}
