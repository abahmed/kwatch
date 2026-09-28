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
}

// registration pairs a schema with its informer in the shared factory.
type registration struct {
	schema   Schema
	informer func(informers.SharedInformerFactory) cache.SharedIndexInformer
}

func registrations() []registration {
	type factory = informers.SharedInformerFactory
	type informer = cache.SharedIndexInformer
	return []registration{
		{PodSchema{}, func(f factory) informer {
			return f.Core().V1().Pods().Informer()
		}},
		{NodeSchema{}, func(f factory) informer {
			return f.Core().V1().Nodes().Informer()
		}},
		{DeploymentSchema(), func(f factory) informer {
			return f.Apps().V1().Deployments().Informer()
		}},
		{ReplicaSetSchema(), func(f factory) informer {
			return f.Apps().V1().ReplicaSets().Informer()
		}},
		{StatefulSetSchema(), func(f factory) informer {
			return f.Apps().V1().StatefulSets().Informer()
		}},
		{DaemonSetSchema(), func(f factory) informer {
			return f.Apps().V1().DaemonSets().Informer()
		}},
		{JobSchema(), func(f factory) informer {
			return f.Batch().V1().Jobs().Informer()
		}},
		{CronJobSchema{}, func(f factory) informer {
			return f.Batch().V1().CronJobs().Informer()
		}},
		{HPASchema{}, func(f factory) informer {
			return f.Autoscaling().V2().HorizontalPodAutoscalers().
				Informer()
		}},
		{ServiceSchema{}, func(f factory) informer {
			return f.Core().V1().Services().Informer()
		}},
		{EndpointSliceSchema{}, func(f factory) informer {
			return f.Discovery().V1().EndpointSlices().Informer()
		}},
		{IngressSchema{}, func(f factory) informer {
			return f.Networking().V1().Ingresses().Informer()
		}},
		{SecretSchema{}, func(f factory) informer {
			return f.Core().V1().Secrets().Informer()
		}},
		{ConfigMapSchema{}, func(f factory) informer {
			return f.Core().V1().ConfigMaps().Informer()
		}},
		{ServiceAccountSchema{}, func(f factory) informer {
			return f.Core().V1().ServiceAccounts().Informer()
		}},
		{PVCSchema{}, func(f factory) informer {
			return f.Core().V1().PersistentVolumeClaims().Informer()
		}},
		{PVSchema{}, func(f factory) informer {
			return f.Core().V1().PersistentVolumes().Informer()
		}},
		{StorageClassSchema{}, func(f factory) informer {
			return f.Storage().V1().StorageClasses().Informer()
		}},
		{NamespaceSchema{}, func(f factory) informer {
			return f.Core().V1().Namespaces().Informer()
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
		if _, err := informer.AddEventHandler(
			s.handler(NewTranslator(r.schema)),
		); err != nil {
			return nil, err
		}
		s.synced[r.schema.Kind()] = informer.HasSynced
	}
	return s, nil
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

func (s *Source) handler(t *Translator) cache.ResourceEventHandler {
	submit := func(facts []knowledge.Fact) {
		if len(facts) > 0 {
			s.cfg.Submit(context.Background(), facts...)
		}
	}
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, initialList bool) {
			submit(t.Added(obj, initialList, s.cfg.Now()))
		},
		UpdateFunc: func(old, new any) {
			submit(t.Updated(old, new, s.cfg.Now()))
		},
		DeleteFunc: func(obj any) {
			submit(t.Deleted(obj, s.cfg.Now()))
		},
	}
}
