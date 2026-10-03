//go:build e2e

package scenarios

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// secretEnvFrom reads a Pod's environment from the named Secret.
func secretEnvFrom(name string) corev1.EnvFromSource {
	return corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
	}}
}

// configMapEnvFrom reads a Pod's environment from the named ConfigMap.
func configMapEnvFrom(name string) corev1.EnvFromSource {
	return corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
	}}
}

// podWithEnvFrom cannot start until the Secret or ConfigMap it reads its
// environment from exists.
func podWithEnvFrom(name string, from corev1.EnvFromSource) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	pod.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{from}
	return pod
}

// CreateConfigMap creates a small ConfigMap in the scenario namespace.
func (s *Scenario) CreateConfigMap(name string) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().ConfigMaps(s.Namespace).Create(s.Ctx,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Data:       map[string]string{"mode": "ok"},
		}, metav1.CreateOptions{})
	s.Must(err)
}

// DeleteConfigMap deletes a ConfigMap of the scenario namespace.
func (s *Scenario) DeleteConfigMap(name string) {
	s.T.Helper()
	s.Must(s.Env.Client.CoreV1().ConfigMaps(s.Namespace).Delete(
		s.Ctx, name, metav1.DeleteOptions{}))
}

// FixMissingConfigMap creates the missing ConfigMap and replaces the Pods of
// the Deployment. The kubelet retries a Pod that cannot start less and less
// often (up to every five minutes), so without new Pods the fix would reach
// the workload at an unpredictable time.
func (s *Scenario) FixMissingConfigMap(deployment, configMap string) {
	s.T.Helper()
	s.CreateConfigMap(configMap)
	s.RestartPods(deployment)
}

// CreateExpiredTLSSecret creates a TLS Secret whose certificate ended an
// hour ago.
func (s *Scenario) CreateExpiredTLSSecret(name string) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().Secrets(s.Namespace).Create(s.Ctx,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": s.expiredCertificate(),
				"tls.key": []byte("not-used-by-monitor"),
			},
		}, metav1.CreateOptions{})
	s.Must(err)
}

func (s *Scenario) expiredCertificate() []byte {
	s.T.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.Must(err)
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: now.Add(-2 * time.Hour),
		NotAfter: now.Add(-time.Hour), BasicConstraintsValid: true,
		DNSNames: []string{"kwatch-e2e.invalid"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template,
		&key.PublicKey, key)
	s.Must(err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// CreatePodWithDeletedServiceAccount creates the account and a Pod that
// uses it, then deletes the account. The label patch makes the Pod change
// after the deletion, so Kwatch sees the dangling reference.
func (s *Scenario) CreatePodWithDeletedServiceAccount(name string) {
	s.T.Helper()
	core := s.Env.Client.CoreV1()
	_, err := core.ServiceAccounts(s.Namespace).Create(s.Ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{})
	s.Must(err)
	pod := workloadPod(name, "sleep")
	pod.Spec.ServiceAccountName = name
	s.CreatePod(pod)
	s.Must(core.ServiceAccounts(s.Namespace).Delete(
		s.Ctx, name, metav1.DeleteOptions{}))
	label := []byte(`{"metadata":{"labels":{"kwatch-e2e":"` + name + `"}}}`)
	_, err = core.Pods(s.Namespace).Patch(s.Ctx, name,
		types.MergePatchType, label, metav1.PatchOptions{})
	s.Must(err)
}
