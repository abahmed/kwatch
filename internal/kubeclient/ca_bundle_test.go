package kubeclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestCA(t *testing.T) (string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kwatch test ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(
		rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, der
}

func TestApplyCABundleAddsToSystemRoots(t *testing.T) {
	path, der := writeTestCA(t)
	cfg := &tls.Config{}
	applyCABundle(cfg, path)
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs was not set")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cert.Verify(x509.VerifyOptions{
		Roots: cfg.RootCAs, CurrentTime: time.Now(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil {
		t.Fatalf("bundle CA is not trusted: %v", err)
	}
}

func TestApplyCABundleIgnoresBadFile(t *testing.T) {
	cfg := &tls.Config{}
	applyCABundle(cfg, filepath.Join(t.TempDir(), "missing.pem"))
	if cfg.RootCAs != nil {
		t.Fatal("a missing bundle must leave RootCAs unset")
	}
}
