package security

import (
	"fmt"

	corev1lister "k8s.io/client-go/listers/core/v1"
)

// TLSSources contains the informer-backed Secret source for TLS monitoring.
type TLSSources struct {
	Secrets corev1lister.SecretLister
}

// ConfigureSources freezes TLS sources before the first sweep.
func (r *TLSRuntime) ConfigureSources(sources TLSSources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("tls sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("tls sources are already configured")
	}
	r.configured = true
	r.secret = sources.Secrets
	return nil
}
