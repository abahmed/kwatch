package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/model"
)

func TestControllerStatusAndSourceAdapters(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := &Controller{now: func() time.Time { return now }}
	c.recordInformerEvent()
	c.recordInformerWatchError(nil)
	c.recordInformerWatchError(
		apiForbiddenError(),
	)
	if got := safeWatchError(
		errors.New("request timeout"),
	); got != "watch_timeout" {
		t.Fatalf("safeWatchError(timeout) = %q", got)
	}
	if got := safeWatchError(nil); got != "watch_failed" {
		t.Fatalf("safeWatchError(nil) = %q", got)
	}
	if _, err := c.StatusJSON(); err != nil {
		t.Fatalf("StatusJSON() error = %v", err)
	}

	c.pod = newResourcePipeline("pod", "pods")
	c.pod.startWorkers = true
	if got := c.unavailableSources(); len(got) == 0 {
		t.Fatal("unavailableSources() returned no active missing source")
	}
	if len(c.allPipelines()) == 0 {
		t.Fatal("pipeline inventory was empty")
	}

	assertAttributionSourceAdapters(t)
	if got := (&Controller{}).IncidentAttributionSources(); got == nil {
		t.Fatal("IncidentAttributionSources() returned nil")
	}
}

func assertAttributionSourceAdapters(t *testing.T) {
	t.Helper()
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	sources := attributionSources{
		deployments: appsv1lister.NewDeploymentLister(
			indexerWith(t, deployment),
		),
		statefulSet: appsv1lister.NewStatefulSetLister(
			indexerWith(t, statefulSet),
		),
		daemonSet: appsv1lister.NewDaemonSetLister(
			indexerWith(t, daemonSet),
		),
		services: corev1lister.NewServiceLister(
			indexerWith(t, service),
		),
	}
	if got, err := sources.Deployment("apps", "api"); err != nil ||
		got.Name != "api" {
		t.Fatalf("Deployment() = %v, %v", got, err)
	}
	if got, err := sources.Service("apps", "api"); err != nil ||
		got.Name != "api" {
		t.Fatalf("Service() = %v, %v", got, err)
	}
	if got, err := sources.StatefulSet("apps", "api"); err != nil ||
		got.Name != "api" {
		t.Fatalf("StatefulSet() = %v, %v", got, err)
	}
	if got, err := sources.DaemonSet("apps", "api"); err != nil ||
		got.Name != "api" {
		t.Fatalf("DaemonSet() = %v, %v", got, err)
	}
	if got, err := sources.ListServices("apps"); err != nil || len(got) != 1 {
		t.Fatalf("ListServices() = %v, %v", got, err)
	}
	assertEmptyAttributionSources(t)
	var _ incident.AttributionSources = sources
}

func assertEmptyAttributionSources(t *testing.T) {
	t.Helper()
	var empty attributionSources
	if got, err := empty.Service("apps", "missing"); err != nil || got != nil {
		t.Fatalf("nil Service() = %v, %v", got, err)
	}
	if got, err := empty.Deployment("apps", "missing"); err != nil || got != nil {
		t.Fatalf("nil Deployment() = %v, %v", got, err)
	}
	if got, err := empty.StatefulSet("apps", "missing"); err != nil || got != nil {
		t.Fatalf("nil StatefulSet() = %v, %v", got, err)
	}
	if got, err := empty.DaemonSet("apps", "missing"); err != nil || got != nil {
		t.Fatalf("nil DaemonSet() = %v, %v", got, err)
	}
	if got, err := empty.ListServices("apps"); err != nil || got != nil {
		t.Fatalf("nil ListServices() = %v, %v", got, err)
	}
}

func TestControllerAccessorsExposeConfiguredSources(t *testing.T) {
	c := &Controller{}
	if c.PodLister() != nil || c.ServiceLister() != nil ||
		c.NodeLister() != nil {
		t.Fatal("unconfigured lister accessor returned a value")
	}
	if c.OwnerResolver() == nil {
		t.Fatal("OwnerResolver() returned nil")
	}
	c.recordGraphSize()
}

func TestControllerGraphEdgeHelpersHandleSmallInputs(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	c := &Controller{graphRuntime: graphRuntime{graph: graph}}
	c.recordGraphSize()
	c.rebuildNodeGraph("not-a-node")
	c.rebuildLeaseGraph("not-a-lease")
	c.addNodeLeaseEdge("")
	if !podGraphInputsChanged(nil, &corev1.Pod{}) {
		t.Fatal("nil pod input was not treated as changed")
	}
	if containerEnvChanged(&corev1.Container{}, &corev1.Container{}) {
		t.Fatal("equal container environment was marked changed")
	}
	builder := &graphBuilder{graph: graph}
	builder.addContainerEnvToGraph("apps", "api", corev1.Container{
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
			}},
			{SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "sec"},
			}},
		},
		Env: []corev1.EnvVar{
			{Name: "CFG", ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
				},
			}},
			{Name: "SEC", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "sec"},
				},
			}},
		},
	})
}

type attributionConfigCoverageStub struct{}

func (attributionConfigCoverageStub) ConfigureAttributionSources(
	incident.AttributionSources,
) error {
	return nil
}

func TestControllerLifecycleAndEventEdges(t *testing.T) {
	c := &Controller{
		components: RuntimeSet{IncidentSources: attributionConfigCoverageStub{}},
		now:        time.Now,
	}
	if err := configureDirectRuntimes(c, c.components); err != nil {
		t.Fatalf("configureDirectRuntimes() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.seedThresholds.nodeLeaseStaleSeconds = 1
	c.runLeaseSweep(ctx)

	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		eventsByPodIndex: func(obj interface{}) ([]string, error) {
			event := obj.(*corev1.Event)
			return []string{eventsByPodKey(
				event.InvolvedObject.Namespace,
				event.InvolvedObject.Name,
			)}, nil
		},
	})
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{
		Name: "event", Namespace: "apps",
	}, InvolvedObject: corev1.ObjectReference{
		Namespace: "apps", Name: "api",
	}}
	if err := indexer.Add(event); err != nil {
		t.Fatalf("add event: %v", err)
	}
	c.eventIndexers = []cache.Indexer{indexer}
	if events, err := c.eventsByPod(
		"apps", "api",
	); err != nil || len(events) != 1 {
		t.Fatalf("eventsByPod() = %v, %v", events, err)
	}
	c.ingress = newResourcePipeline("ingress", "ingresses")
	c.mwc = newResourcePipeline("mwc", "mwc")
	c.vwc = newResourcePipeline("vwc", "vwc")
	c.enqueueServiceDependents("unexpected")
	_ = wireClusterAutoscaler(
		&mockHandler{}, fake.NewSimpleClientset(), 0,
	)
	factory := informers.NewSharedInformerFactory(
		fake.NewSimpleClientset(), 0,
	)
	c.listen(c.ingress, factory.Networking().V1().Ingresses().Informer())
	_ = c.wireTLS(fake.NewSimpleClientset(), 0, namespaceScope{all: true})
	c.cpPod = newResourcePipeline("controlplane pod", "pods")
	_ = c.wireControlPlane(fake.NewSimpleClientset(), 0)
	obs := &model.Observation{Subject: model.ObjectRef{
		Kind: "pod", Namespace: "apps", Name: "api",
	}}
	newBaselineRecorder(time.Now(), 2).seedControlPlane(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api"}}, obs,
	)
	WatchNamespaceScope(context.Background(), nil, "", nil, nil)
}

func TestControllerSyncDispatchCoversPipelines(t *testing.T) {
	h := &mockHandler{}
	endpointIndexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, nil,
	)
	cpIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
	c := &Controller{
		components: componentsFor(h),
		endpointSliceLister: discoveryv1lister.NewEndpointSliceLister(
			endpointIndexer,
		),
		cpPodLister: corev1lister.NewPodLister(cpIndexer),
	}
	ctx := context.Background()
	checks := []struct {
		name string
		call func() error
	}{
		{"pod", func() error { return c.syncPod(ctx, "apps/api") }},
		{"node", func() error { return c.syncNode(ctx, "node-a") }},
		{"deployment", func() error { return c.syncDeployment(ctx, "apps/api") }},
		{"job", func() error { return c.syncJob(ctx, "apps/api") }},
		{"daemonset", func() error { return c.syncDaemonSet(ctx, "apps/api") }},
		{"statefulset", func() error { return c.syncStatefulSet(ctx, "apps/api") }},
		{"pdb", func() error { return c.syncPdb(ctx, "apps/api") }},
		{"cronjob", func() error { return c.syncCronJob(ctx, "apps/api") }},
		{"hpa", func() error {
			return c.syncHorizontalPodAutoscaler(ctx, "apps/api")
		}},
		{"service", func() error { return c.syncService(ctx, "apps/api") }},
		{"endpoint-slice", func() error {
			return c.syncEndpointSlice(ctx, "apps/api")
		}},
		{"mwc", func() error { return c.syncMwc(ctx, "api") }},
		{"vwc", func() error { return c.syncVwc(ctx, "api") }},
		{"ingress", func() error { return c.syncIngress(ctx, "apps/api") }},
		{"netpol", func() error { return c.syncNetpol(ctx, "apps/api") }},
		{"control-plane", func() error { return c.syncCpPod(ctx, "apps/api") }},
		{"resource-quota", func() error {
			return c.syncResourceQuota(ctx, "apps/api")
		}},
		{"limit-range", func() error { return c.syncLimitRange(ctx, "apps/api") }},
		{"namespace", func() error { return c.syncNamespace(ctx, "apps") }},
		{"lease", func() error { return c.syncLease(ctx, "apps/api") }},
		{"replicaset", func() error { return c.syncReplicaSet(ctx, "apps/api") }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err != nil {
				t.Fatalf("sync returned error: %v", err)
			}
		})
	}
}

func TestNamespaceSelectionHelpers(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "z", Labels: map[string]string{"team": "api"},
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "a", Labels: map[string]string{"team": "api"},
		}},
	)
	got, err := selectedNamespaces(
		context.Background(), client, "team=api",
	)
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("selectedNamespaces() = %v, %v", got, err)
	}
	if !sameNamespaces([]string{"a", "z"}, got) {
		t.Fatal("sameNamespaces() rejected equal slices")
	}
	if sameNamespaces([]string{"a"}, got) {
		t.Fatal("sameNamespaces() accepted different lengths")
	}
	if sameNamespaces([]string{"b", "z"}, got) {
		t.Fatal("sameNamespaces() accepted different values")
	}
}

func indexerWith(t *testing.T, objects ...interface{}) cache.Indexer {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
	for _, object := range objects {
		if err := indexer.Add(object); err != nil {
			t.Fatalf("add test object: %v", err)
		}
	}
	return indexer
}

func apiForbiddenError() error {
	return apierrors.NewForbidden(
		corev1.Resource("pods"), "api", nil,
	)
}
