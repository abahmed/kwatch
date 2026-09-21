package controller

import (
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestFactorySetAccessorsExposeEveryInformerFamily(t *testing.T) {
	client := fake.NewSimpleClientset()
	set, factories := newFactories(
		client, namespaceScope{all: true}, nil, time.Minute,
	)
	if len(factories) != 3 {
		t.Fatalf("factory count = %d, want 3", len(factories))
	}
	assertFactoryCoreAccessors(t, set)
	assertFactorySupportAccessors(t, set)
	assertFactoryClusterAccessors(t, set)
}

func assertFactoryCoreAccessors(t *testing.T, set factorySet) {
	t.Helper()
	assertFactoryPodAccessors(t, set)
	assertFactoryNetworkAccessors(t, set)
}

func assertFactoryPodAccessors(t *testing.T, set factorySet) {
	t.Helper()
	assertFactoryBaseAccessors(t, set)
	assertFactoryWorkloadAccessors(t, set)
}

func assertFactoryBaseAccessors(t *testing.T, set factorySet) {
	t.Helper()
	if set.podLister() == nil || len(set.podInformers()) != 1 ||
		set.nodeLister() == nil || set.nodeInformer() == nil ||
		set.deployLister() == nil || len(set.deployInformers()) != 1 ||
		set.jobLister() == nil || set.rsLister() == nil ||
		len(set.rsInformers()) != 1 {
		t.Fatal("base factory accessors did not expose all families")
	}
}

func assertFactoryWorkloadAccessors(t *testing.T, set factorySet) {
	t.Helper()
	if set.dsLister() == nil ||
		len(set.dsInformers()) != 1 || set.ssLister() == nil ||
		len(set.ssInformers()) != 1 || set.pdbLister() == nil ||
		len(set.pdbInformers()) != 1 || len(set.jobInformers()) != 1 ||
		set.cronJobLister() == nil || len(set.cronJobInformers()) != 1 ||
		set.hpaLister() == nil || len(set.hpaInformers()) != 1 ||
		set.serviceLister() == nil || len(set.serviceInformers()) != 1 {
		t.Fatal("global factory accessors did not expose all families")
	}
}

func assertFactoryNetworkAccessors(t *testing.T, set factorySet) {
	t.Helper()
	if set.endpointSliceLister() == nil ||
		len(set.endpointSliceInformers()) != 1 {
		t.Fatal("network factory accessors did not expose all families")
	}
}

func assertFactorySupportAccessors(t *testing.T, set factorySet) {
	t.Helper()
	if set.leaseLister() == nil || len(set.leaseInformers()) != 1 ||
		set.ingressLister() == nil || len(set.ingressInformers()) != 1 ||
		set.netpolLister() == nil || len(set.netpolInformers()) != 1 ||
		set.configMapLister() == nil ||
		len(set.configMapInformers()) != 1 || set.secretLister() == nil ||
		len(set.secretInformers()) != 1 || len(set.pvcInformers()) != 1 ||
		set.pvcLister() == nil || set.serviceAccountLister() == nil ||
		len(set.serviceAccountInformers()) != 1 ||
		set.resourceQuotaLister() == nil ||
		len(set.resourceQuotaInformers()) != 1 ||
		set.limitRangeLister() == nil || len(set.limitRangeInformers()) != 1 {
		t.Fatal("support factory accessors did not expose all families")
	}
}

func assertFactoryClusterAccessors(t *testing.T, set factorySet) {
	t.Helper()
	if set.namespaceLister() == nil || set.namespaceInformer() == nil ||
		set.persistentVolumeLister() == nil ||
		len(set.persistentVolumeInformers()) != 1 ||
		set.storageClassLister() == nil ||
		len(set.storageClassInformers()) != 1 || set.mwcLister() == nil ||
		set.mwcInformer() == nil || set.vwcLister() == nil ||
		set.vwcInformer() == nil {
		t.Fatal("cluster factory accessors did not expose all families")
	}
}

func TestFactorySetAccessorsBuildMultiNamespaceViews(t *testing.T) {
	set, factories := newFactories(
		fake.NewSimpleClientset(),
		namespaceScope{namespaces: []string{"a", "b"}}, nil,
		time.Minute,
	)
	if len(factories) != 4 || len(set.perNamespace) != 2 {
		t.Fatalf("multi-namespace factories = %d/%d", len(factories),
			len(set.perNamespace))
	}
	if set.podLister() == nil || len(set.podInformers()) != 2 ||
		set.deployLister() == nil || len(set.deployInformers()) != 2 ||
		set.serviceLister() == nil || len(set.serviceInformers()) != 2 ||
		set.mwcLister() == nil || set.vwcLister() == nil {
		t.Fatal("multi-namespace accessors did not build aggregate views")
	}
}
