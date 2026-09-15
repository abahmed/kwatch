package security

import corev1lister "k8s.io/client-go/listers/core/v1"

// SetSources adapts the retired setter-style wiring for compatibility tests
// and embedded callers. Production composition uses ConfigureSources.
func (r *Runtime) SetSources(sources Sources) {
	_ = r.ConfigureSources(sources)
}

// SetSecretLister preserves the historical TLS setter for compatibility
// callers. Production composition uses TLSRuntime.ConfigureSources.
func (r *TLSRuntime) SetSecretLister(lister corev1lister.SecretLister) {
	_ = r.ConfigureSources(TLSSources{Secrets: lister})
}
