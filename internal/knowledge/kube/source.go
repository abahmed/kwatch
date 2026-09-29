package kube

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Submit delivers facts to the pipeline.
type Submit func(ctx context.Context, facts ...knowledge.Fact)

// SourceConfig configures the Kubernetes source.
type SourceConfig struct {
	Client kubernetes.Interface
	Resync time.Duration
	Now    func() time.Time
	Submit Submit
	// Maintenance names the maintenance annotations; zero disables them.
	Maintenance MaintenanceAnnotations
}

// registration pairs a schema with its informer in the shared factory and
// the API resource the informer lists and watches.
type registration struct {
	resource Resource
	schema   Schema
	informer func(informers.SharedInformerFactory) cache.SharedIndexInformer
}

func registrations() []registration {
	type factory = informers.SharedInformerFactory
	type informer = cache.SharedIndexInformer
	const admission = "admissionregistration.k8s.io"
	return []registration{
		{Resource{"", "pods"}, PodSchema{},
			func(f factory) informer {
				return f.Core().V1().Pods().Informer()
			}},
		{Resource{"", "nodes"}, NodeSchema{},
			func(f factory) informer {
				return f.Core().V1().Nodes().Informer()
			}},
		{Resource{"apps", "deployments"}, DeploymentSchema(),
			func(f factory) informer {
				return f.Apps().V1().Deployments().Informer()
			}},
		{Resource{"apps", "replicasets"}, ReplicaSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().ReplicaSets().Informer()
			}},
		{Resource{"apps", "statefulsets"}, StatefulSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().StatefulSets().Informer()
			}},
		{Resource{"apps", "daemonsets"}, DaemonSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().DaemonSets().Informer()
			}},
		{Resource{"batch", "jobs"}, JobSchema(),
			func(f factory) informer {
				return f.Batch().V1().Jobs().Informer()
			}},
		{Resource{"batch", "cronjobs"}, CronJobSchema{},
			func(f factory) informer {
				return f.Batch().V1().CronJobs().Informer()
			}},
		{Resource{"autoscaling", "horizontalpodautoscalers"}, HPASchema{},
			func(f factory) informer {
				return f.Autoscaling().V2().HorizontalPodAutoscalers().
					Informer()
			}},
		{Resource{"", "services"}, ServiceSchema{},
			func(f factory) informer {
				return f.Core().V1().Services().Informer()
			}},
		{Resource{"discovery.k8s.io", "endpointslices"}, EndpointSliceSchema{},
			func(f factory) informer {
				return f.Discovery().V1().EndpointSlices().Informer()
			}},
		{Resource{"networking.k8s.io", "ingresses"}, IngressSchema{},
			func(f factory) informer {
				return f.Networking().V1().Ingresses().Informer()
			}},
		{Resource{"", "secrets"}, SecretSchema{},
			func(f factory) informer {
				return f.Core().V1().Secrets().Informer()
			}},
		{Resource{"", "configmaps"}, ConfigMapSchema{},
			func(f factory) informer {
				return f.Core().V1().ConfigMaps().Informer()
			}},
		{Resource{"", "serviceaccounts"}, ServiceAccountSchema{},
			func(f factory) informer {
				return f.Core().V1().ServiceAccounts().Informer()
			}},
		{Resource{"", "persistentvolumeclaims"}, PVCSchema{},
			func(f factory) informer {
				return f.Core().V1().PersistentVolumeClaims().Informer()
			}},
		{Resource{"", "persistentvolumes"}, PVSchema{},
			func(f factory) informer {
				return f.Core().V1().PersistentVolumes().Informer()
			}},
		{Resource{"storage.k8s.io", "storageclasses"}, StorageClassSchema{},
			func(f factory) informer {
				return f.Storage().V1().StorageClasses().Informer()
			}},
		{Resource{"", "namespaces"}, NamespaceSchema{},
			func(f factory) informer {
				return f.Core().V1().Namespaces().Informer()
			}},
		{Resource{"", "limitranges"}, LimitRangeSchema{},
			func(f factory) informer {
				return f.Core().V1().LimitRanges().Informer()
			}},
		{Resource{"policy", "poddisruptionbudgets"}, PDBSchema{},
			func(f factory) informer {
				return f.Policy().V1().PodDisruptionBudgets().Informer()
			}},
		{Resource{"", "resourcequotas"}, QuotaSchema{},
			func(f factory) informer {
				return f.Core().V1().ResourceQuotas().Informer()
			}},
		{Resource{"networking.k8s.io", "networkpolicies"}, NetworkPolicySchema{},
			func(f factory) informer {
				return f.Networking().V1().NetworkPolicies().Informer()
			}},
		{Resource{"storage.k8s.io", "volumeattachments"},
			VolumeAttachmentSchema{},
			func(f factory) informer {
				return f.Storage().V1().VolumeAttachments().Informer()
			}},
		{Resource{admission, "mutatingwebhookconfigurations"},
			WebhookSchema{Mutating: true},
			func(f factory) informer {
				return f.Admissionregistration().V1().
					MutatingWebhookConfigurations().Informer()
			}},
		{Resource{admission, "validatingwebhookconfigurations"},
			WebhookSchema{},
			func(f factory) informer {
				return f.Admissionregistration().V1().
					ValidatingWebhookConfigurations().Informer()
			}},
	}
}

// transform keeps informer caches small and free of secret material.
func transform(obj any) (any, error) {
	obj, err := TrimToLatestManager(obj)
	if err != nil {
		return obj, err
	}
	if _, ok := obj.(*corev1.Secret); ok {
		return HashSecretData(obj)
	}
	return obj, nil
}

// Source streams facts from Kubernetes informers.
type Source struct {
	cfg     SourceConfig
	factory informers.SharedInformerFactory
	synced  map[knowledge.Kind]cache.InformerSynced
	started chan struct{}
}

// NewSource registers an informer and handler for every schema.
func NewSource(cfg SourceConfig) (*Source, error) {
	if cfg.Client == nil || cfg.Submit == nil || cfg.Now == nil {
		return nil, errors.New("kube source: incomplete configuration")
	}
	factory := informers.NewSharedInformerFactoryWithOptions(
		cfg.Client, cfg.Resync, informers.WithTransform(transform),
	)
	s := &Source{
		cfg: cfg, factory: factory,
		synced:  make(map[knowledge.Kind]cache.InformerSynced),
		started: make(chan struct{}),
	}
	for _, r := range registrations() {
		informer := r.informer(factory)
		if _, err := informer.AddEventHandler(translatorHandler(
			NewTranslator(r.schema).WithMaintenance(cfg.Maintenance),
			cfg.Submit, cfg.Now),
		); err != nil {
			return nil, err
		}
		s.synced[r.schema.Kind()] = informer.HasSynced
	}
	events := factory.Core().V1().Events().Informer()
	if _, err := events.AddEventHandler(s.eventHandler()); err != nil {
		return nil, err
	}
	return s, nil
}

// eventHandler records Warning events as notes. Deleting an Event object
// (expiry) keeps the note: the evidence outlives the event.
func (s *Source) eventHandler() cache.ResourceEventHandler {
	note := func(obj any) {
		if fact, ok := EventNote(obj, s.cfg.Now()); ok {
			s.cfg.Submit(context.Background(), fact)
		}
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    note,
		UpdateFunc: func(_, new any) { note(new) },
	}
}

// Run starts the informers and blocks until ctx ends and every informer
// has stopped.
func (s *Source) Run(ctx context.Context) {
	s.factory.Start(ctx.Done())
	close(s.started)
	<-ctx.Done()
	s.factory.Shutdown()
}

// WaitForSync blocks until every informer completed its initial list or
// ctx ends, and reports whether all of them synced.
func (s *Source) WaitForSync(ctx context.Context) bool {
	select {
	case <-s.started:
	case <-ctx.Done():
		return false
	}
	for _, ok := range s.factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return false
		}
	}
	return true
}

// Synced reports whether a kind has completed its initial list. Kinds
// without an informer are never synced.
func (s *Source) Synced(kind knowledge.Kind) bool {
	fn, ok := s.synced[kind]
	if !ok {
		// Containers are described with their pods.
		if kind == KindContainer {
			fn, ok = s.synced[KindPod]
		}
		if !ok {
			return false
		}
	}
	return fn()
}

// translatorHandler submits the facts a translator produces for informer
// notifications.
func translatorHandler(
	t *Translator, submit Submit, now func() time.Time,
) cache.ResourceEventHandler {
	send := func(facts []knowledge.Fact) {
		if len(facts) > 0 {
			submit(context.Background(), facts...)
		}
	}
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, initialList bool) {
			send(t.Added(obj, initialList, now()))
		},
		UpdateFunc: func(old, new any) {
			send(t.Updated(old, new, now()))
		},
		DeleteFunc: func(obj any) {
			send(t.Deleted(obj, now()))
		},
	}
}
