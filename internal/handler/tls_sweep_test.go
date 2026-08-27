package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func generateTestCert(
	t *testing.T,
	commonName string,
	notAfter time.Time,
) []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotAfter:     notAfter,
	}
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&key.PublicKey,
		key,
	)
	assert.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func newHandlerForTest() *handler {
	return NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		correlation.NewEngine(correlation.Config{}),
		testAlertMgr,
	)
}

func TestSweepTLSSecretsNilLister(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		correlation.NewEngine(correlation.Config{}),
		testAlertMgr,
	)
	h.SweepTLSSecrets()
}

func TestCheckTLSSecretNoCertData(t *testing.T) {
	h := newHandlerForTest()
	secret := &corev1.Secret{Data: map[string][]byte{}}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
}

func TestCheckTLSSecretExpired(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(-24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestCheckTLSSecretExpiringSoon(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(10*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestCheckTLSSecretExpiringSoonCritical(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(2*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestCheckTLSSecretHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(365*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
	assert.Equal(t, 0, e.ActiveCount())
}

func TestCheckTLSSecretPEMDecodeFail(t *testing.T) {
	h := newHandlerForTest()
	secret := &corev1.Secret{
		Data: map[string][]byte{"tls.crt": []byte("not-valid-pem-data")},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
}

func TestCheckTLSSecretCertParseFail(t *testing.T) {
	h := newHandlerForTest()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"tls.crt": pem.EncodeToMemory(
				&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid-der")},
			),
		},
	}
	h.checkTLSSecret(secret, time.Now(), 30*24*time.Hour)
}

func TestSweepTLSSecretsWithLister(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(10*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Secrets().Informer().GetIndexer().Add(secret)
	h.listers.Secret = f.Core().V1().Secrets().Lister()
	h.SweepTLSSecrets()
	assert.Equal(
		t,
		1,
		e.ActiveCount(),
		"expiring secret should create incident via sweep",
	)
}

func TestSweepTLSSecretsWithThresholdDefault(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		TlsMonitor: config.TlsMonitor{Threshold: 0},
	}, e, testAlertMgr)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(10*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Secrets().Informer().GetIndexer().Add(secret)
	h.listers.Secret = f.Core().V1().Secrets().Lister()
	h.SweepTLSSecrets()
	assert.Equal(t, 1, e.ActiveCount())
}

func TestSweepTLSSecretsExpired(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(-24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Secrets().Informer().GetIndexer().Add(secret)
	h.listers.Secret = f.Core().V1().Secrets().Lister()
	h.SweepTLSSecrets()
	assert.Equal(t, 1, e.ActiveCount())
}

func TestSweepTLSSecretsHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	certData := generateTestCert(
		t,
		"test.example.com",
		time.Now().Add(365*24*time.Hour),
	)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls1", Namespace: "ns1"},
		Data:       map[string][]byte{"tls.crt": certData},
	}
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Secrets().Informer().GetIndexer().Add(secret)
	h.listers.Secret = f.Core().V1().Secrets().Lister()
	h.SweepTLSSecrets()
	assert.Equal(t, 0, e.ActiveCount())
}
