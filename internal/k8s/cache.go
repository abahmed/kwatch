package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrimManagedFields removes server-side apply ownership metadata before an
// object enters an informer cache. The transform runs before publication, so
// mutating the object in place is safe and avoids an extra deep copy.
func TrimManagedFields(obj interface{}) (interface{}, error) {
	if meta, ok := obj.(metav1.Object); ok {
		meta.SetManagedFields(nil)
	}
	if secret, ok := obj.(*corev1.Secret); ok {
		stripSecretData(secret)
	}
	return obj, nil
}

// stripSecretData keeps only the public TLS certificate. Kwatch checks Secret
// existence and certificate expiry; private keys and other values must never
// sit in its cache.
func stripSecretData(secret *corev1.Secret) {
	cert := secret.Data[corev1.TLSCertKey]
	secret.Data = nil
	secret.StringData = nil
	if secret.Type == corev1.SecretTypeTLS && len(cert) > 0 {
		secret.Data = map[string][]byte{corev1.TLSCertKey: cert}
	}
	delete(secret.Annotations, corev1.LastAppliedConfigAnnotation)
}
