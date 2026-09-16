package security

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

type tlsSink struct {
	observations []*model.Observation
}

func (s *tlsSink) Process(
	observation *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.observations = append(s.observations, observation)
	return nil, model.ActionSkip
}

func (s *tlsSink) Resolve(model.ObjectRef, string) {}

func (s *tlsSink) ResolveObserved(*model.Observation) {}

func (s *tlsSink) Reconcile(
	_ model.ObjectRef,
	observations []*model.Observation,
) {
	for _, observation := range observations {
		if observation != nil {
			s.observations = append(s.observations, observation)
		}
	}
}

func (s *tlsSink) ReconcileGone(model.ObjectRef) {}

func TestTLSRuntimeSkipsWithoutConfiguration(t *testing.T) {
	runtime := NewTLSRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &tlsSink{}, time.Now,
	)
	runtime.SweepTLSSecrets()
}

func TestTLSRuntimeUsesCachedSecrets(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := &config.Config{}
	sink := &tlsSink{}
	runtime := NewTLSRuntimeWithRuntimeConfig(
		config.RuntimeConfigFor(cfg), sink, func() time.Time { return now },
	)
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	if err := indexer.Add(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "demo"},
		Data:       map[string][]byte{"tls.crt": []byte("not pem")},
	}); err != nil {
		t.Fatal(err)
	}
	err := runtime.ConfigureSources(TLSSources{
		Secrets: corev1lister.NewSecretLister(indexer),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.SweepTLSSecrets()
	if len(sink.observations) != 0 {
		t.Fatalf(
			"got %d observations for invalid PEM, want none",
			len(sink.observations),
		)
	}
}
