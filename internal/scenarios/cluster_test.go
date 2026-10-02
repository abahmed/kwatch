package scenarios

import (
	"fmt"
	"sort"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// cluster records what a simulated cluster's informers would deliver. It
// keeps the last version of every object so an update is translated as
// the informer would see it: old and new. Every name a scenario creates
// goes through n, so the same scenario can run many times in one log (the
// staging day) without two instances sharing an object.
type cluster struct {
	start time.Time
	now   time.Time
	// suffix is appended to every name n returns. Empty for the labelled
	// scenario logs.
	suffix  string
	entries []replay.Entry
	objects map[string]runtime.Object
	// probed lists the virtual entities probe reported on.
	probed []inventory.EntityID
}

func newCluster(start time.Time, suffix string) *cluster {
	return &cluster{
		start: start, now: start, suffix: suffix,
		objects: make(map[string]runtime.Object),
	}
}

// n returns the instance name of a scenario name.
func (c *cluster) n(name string) string { return name + c.suffix }

// after moves the simulated clock forward.
func (c *cluster) after(d time.Duration) { c.now = c.now.Add(d) }

// list delivers objects as part of the informers' initial list: kwatch
// sees them for the first time, and no change is recorded.
func (c *cluster) list(objects ...runtime.Object) {
	for _, obj := range objects {
		c.remember(obj)
		c.emit(translator(obj).Added(obj, true, c.now)...)
	}
}

// create delivers genuinely new objects, recording their creation.
func (c *cluster) create(objects ...runtime.Object) {
	for _, obj := range objects {
		c.remember(obj)
		c.emit(translator(obj).Added(obj, false, c.now)...)
	}
}

// update delivers a new version of objects seen before. An object never
// seen is created.
func (c *cluster) update(objects ...runtime.Object) {
	for _, obj := range objects {
		old, ok := c.objects[objectKey(obj)]
		if !ok {
			c.create(obj)
			continue
		}
		c.remember(obj)
		c.emit(translator(obj).Updated(old, obj, c.now)...)
	}
}

// remove delivers deletions.
func (c *cluster) remove(objects ...runtime.Object) {
	for _, obj := range objects {
		delete(c.objects, objectKey(obj))
		c.emit(translator(obj).Deleted(obj, c.now)...)
	}
}

// warn delivers Warning events.
func (c *cluster) warn(events ...*corev1.Event) {
	for _, ev := range events {
		if observation, ok := kube.EventNote(ev, c.now); ok {
			c.emit(observation)
		}
	}
}

// probe records one result of kwatch's own probe of a virtual entity such
// as cluster DNS. An empty failure is a healthy probe.
func (c *cluster) probe(id inventory.EntityID, failure string) {
	attrs := map[string]inventory.Value{
		kube.AttrHealthy:   inventory.Bool(failure == ""),
		kube.AttrLatencyMS: inventory.Number(3),
	}
	if failure != "" {
		attrs[kube.AttrProbeError] = inventory.Text(failure)
	}
	c.probed = append(c.probed, id)
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ProbeSource, At: c.now,
		Entity: id, Attributes: attrs,
	})
}

// last returns a copy of the last version of an object, for editing.
func last[T runtime.Object](c *cluster, obj T) T {
	stored, ok := c.objects[objectKey(obj)]
	if !ok {
		return obj
	}
	return stored.DeepCopyObject().(T)
}

func (c *cluster) emit(observations ...inventory.Observation) {
	sortRelations(observations)
	at := c.now.UTC()
	for _, observation := range observations {
		c.entries = append(c.entries,
			replay.Entry{At: at, Observation: observation})
	}
}

// sortRelations orders each run of Related observations of one entity by
// relation type. The translator emits a child's relations in map order;
// sorting keeps committed logs byte-for-byte reproducible. The model
// replaces each relation type on its own, so the order carries no meaning.
func sortRelations(observations []inventory.Observation) {
	for start := 0; start < len(observations); {
		end := start + 1
		for end < len(observations) &&
			sameRelationRun(observations[start], observations[end]) {
			end++
		}
		run := observations[start:end]
		sort.SliceStable(run, func(i, j int) bool {
			return run[i].Relation < run[j].Relation
		})
		start = end
	}
}

func sameRelationRun(first, next inventory.Observation) bool {
	return first.Kind == inventory.Related &&
		next.Kind == inventory.Related && first.Entity == next.Entity
}

func (c *cluster) remember(obj runtime.Object) {
	c.objects[objectKey(obj)] = obj.DeepCopyObject()
}

// log returns the recorded observations as a replay log.
func (c *cluster) log() replay.Log {
	return replay.Log{Start: c.start.UTC(), Entries: c.entries}
}

// merge interleaves logs recorded independently into one, in time order.
// Entries at the same time keep the order of their logs.
func merge(start time.Time, logs ...replay.Log) replay.Log {
	var entries []replay.Entry
	for _, log := range logs {
		entries = append(entries, log.Entries...)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].At.Before(entries[j].At)
	})
	return replay.Log{Start: start.UTC(), Entries: entries}
}

func objectKey(obj runtime.Object) string {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.GetKind() + "/" + u.GetNamespace() + "/" + u.GetName()
	}
	meta, ok := obj.(interface {
		GetNamespace() string
		GetName() string
	})
	if !ok {
		panic(fmt.Sprintf("scenarios: %T has no object metadata", obj))
	}
	return fmt.Sprintf("%T/%s/%s", obj, meta.GetNamespace(), meta.GetName())
}

// translator picks the schema the application uses for an object type.
func translator(obj runtime.Object) *kube.Translator {
	return kube.NewTranslator(schemaFor(obj))
}

func schemaFor(obj runtime.Object) kube.Schema {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return kube.NewUnstructuredSchema(
			u.GroupVersionKind().Group, u.GetKind())
	}
	if schema, ok := workloadSchemaFor(obj); ok {
		return schema
	}
	switch obj.(type) {
	case *corev1.Node:
		return kube.NodeSchema{}
	case *corev1.Pod:
		return kube.PodSchema{}
	case *corev1.Service:
		return kube.ServiceSchema{}
	case *discoveryv1.EndpointSlice:
		return kube.EndpointSliceSchema{}
	case *networkingv1.Ingress:
		return kube.IngressSchema{}
	case *networkingv1.NetworkPolicy:
		return kube.NetworkPolicySchema{}
	case *corev1.Secret:
		return kube.SecretSchema{}
	case *corev1.ConfigMap:
		return kube.ConfigMapSchema{}
	case *corev1.ServiceAccount:
		return kube.ServiceAccountSchema{}
	}
	return otherSchemaFor(obj)
}

func workloadSchemaFor(obj runtime.Object) (kube.Schema, bool) {
	switch obj.(type) {
	case *appsv1.Deployment:
		return kube.DeploymentSchema(), true
	case *appsv1.ReplicaSet:
		return kube.ReplicaSetSchema(), true
	case *appsv1.StatefulSet:
		return kube.StatefulSetSchema(), true
	case *appsv1.DaemonSet:
		return kube.DaemonSetSchema(), true
	case *batchv1.Job:
		return kube.JobSchema(), true
	case *batchv1.CronJob:
		return kube.CronJobSchema{}, true
	case *autoscalingv2.HorizontalPodAutoscaler:
		return kube.HPASchema{}, true
	}
	return nil, false
}

func otherSchemaFor(obj runtime.Object) kube.Schema {
	switch obj.(type) {
	case *corev1.PersistentVolumeClaim:
		return kube.PVCSchema{}
	case *corev1.PersistentVolume:
		return kube.PVSchema{}
	case *storagev1.StorageClass:
		return kube.StorageClassSchema{}
	case *corev1.Namespace:
		return kube.NamespaceSchema{}
	case *corev1.ResourceQuota:
		return kube.QuotaSchema{}
	case *corev1.LimitRange:
		return kube.LimitRangeSchema{}
	case *policyv1.PodDisruptionBudget:
		return kube.PDBSchema{}
	case *storagev1.VolumeAttachment:
		return kube.VolumeAttachmentSchema{}
	case *admissionv1.ValidatingWebhookConfiguration:
		return kube.WebhookSchema{}
	case *admissionv1.MutatingWebhookConfiguration:
		return kube.WebhookSchema{Mutating: true}
	}
	panic(fmt.Sprintf("scenarios: no schema for %T", obj))
}
