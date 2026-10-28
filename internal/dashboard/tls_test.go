package dashboard

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePair(t *testing.T, dir, name string, at time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1),
		Subject:  pkix.Name{CommonName: name},
		NotAfter: testEpoch.Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl,
		&key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	files := map[string][]byte{
		"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE",
			Bytes: der}),
		"tls.key": pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY",
			Bytes: keyDER}),
	}
	for file, body := range files {
		p := filepath.Join(dir, file)
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCertReloaderServesARenewedCertificate(t *testing.T) {
	dir := t.TempDir()
	writePair(t, dir, "first", testEpoch)
	c := &certReloader{cert: filepath.Join(dir, "tls.crt"),
		key: filepath.Join(dir, "tls.key")}
	first, err := c.get(nil)
	if err != nil {
		t.Fatal(err)
	}
	writePair(t, dir, "second", testEpoch.Add(time.Hour))
	second, err := c.get(nil)
	if err != nil || second == first {
		t.Fatalf("renewal not picked up: %v", err)
	}
	if err := os.WriteFile(c.key, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := testEpoch.Add(2 * time.Hour)
	_ = os.Chtimes(c.key, later, later)
	if kept, err := c.get(nil); err != nil || kept != second {
		t.Fatalf("a broken renewal must keep the last pair: %v", err)
	}
	missing := &certReloader{cert: "/nope/tls.crt", key: "/nope/tls.key"}
	if _, err := missing.get(nil); err == nil {
		t.Fatal("missing files must fail")
	}
}
