package security

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
)

// TLSRuntime owns the periodic TLS Secret sweep. Certificate policy remains
// in DetectTLSSecretIssue so it can be tested without informer state.
type TLSRuntime struct {
	runtime    config.RuntimeConfig
	sink       monitor.ReconciliationSink
	now        func() time.Time
	secret     corev1lister.SecretLister
	mu         sync.Mutex
	started    bool
	configured bool
}

// beginProcessing closes the source configuration window for TLS sweeps.
func (r *TLSRuntime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// SweepTLSSecrets evaluates all cached TLS Secrets.
func (r *TLSRuntime) SweepTLSSecrets() error {
	r.beginProcessing()
	r.mu.Lock()
	secretLister := r.secret
	r.mu.Unlock()
	if secretLister == nil || !r.runtime.Compiled() {
		return nil
	}
	policy := r.runtime.TlsMonitor()
	threshold := policy.Threshold
	if threshold <= 0 {
		threshold = 30
	}
	criticalDays := policy.CriticalThreshold
	warnWindow := time.Duration(threshold) * 24 * time.Hour
	secrets, err := secretLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "tls sweep: failed to list secrets from cache")
		return err
	}
	for _, secret := range secrets {
		r.processSecret(secret, warnWindow, criticalDays)
	}
	return nil
}

func (r *TLSRuntime) processSecret(
	secret *corev1.Secret,
	warnWindow time.Duration,
	criticalDays int,
) {
	observation, err := DetectTLSSecretIssue(
		secret, r.now(), warnWindow, criticalDays,
	)
	if err != nil {
		klog.ErrorS(
			err,
			"tls sweep: parse certificate",
			"secret", secret.Name,
			"namespace", secret.Namespace,
		)
		return
	}
	if r.sink == nil {
		return
	}
	r.prepare(observation)
	r.sink.Reconcile(
		model.NewObjectRef("secret", secret.Namespace, secret.Name),
		[]*model.Observation{observation},
	)
}

func (r *TLSRuntime) prepare(observation *model.Observation) {
	if observation == nil {
		return
	}
	observation.IncludeEvents = r.runtime.IncludeEvents()
	observation.IncludeLogs = r.runtime.IncludeLogs()
}
