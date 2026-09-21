package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
)

func TestSecurityDescriptorIsStable(t *testing.T) {
	descriptor := Descriptor()
	if descriptor.Name != "security-monitor" {
		t.Fatalf("descriptor name = %q", descriptor.Name)
	}
	if len(descriptor.Resources) != 5 {
		t.Fatalf("descriptor resources = %d", len(descriptor.Resources))
	}
}

func TestDetectTLSSecretIssueCoversCertificateStates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		notAfter   time.Time
		warnWindow time.Duration
		critical   int
		wantReason string
		wantHigh   bool
	}{
		{
			name:       "expired",
			notAfter:   now.Add(-time.Hour),
			warnWindow: time.Hour,
			wantReason: "TLSCertExpired",
			wantHigh:   true,
		},
		{
			name:       "critical",
			notAfter:   now.Add(24 * time.Hour),
			warnWindow: 48 * time.Hour,
			critical:   3,
			wantReason: "TLSCertExpiringSoon",
			wantHigh:   true,
		},
		{
			name:       "normal",
			notAfter:   now.Add(5 * 24 * time.Hour),
			warnWindow: 7 * 24 * time.Hour,
			critical:   3,
			wantReason: "TLSCertExpiringSoon",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: test.name},
				Data: map[string][]byte{
					"tls.crt": certificatePEM(t, test.notAfter),
				},
			}
			finding, err := DetectTLSSecretIssue(
				secret, now, test.warnWindow, test.critical,
			)
			if err != nil {
				t.Fatalf("detect certificate: %v", err)
			}
			if finding == nil || finding.Reason != test.wantReason {
				t.Fatalf("finding = %#v", finding)
			}
			if test.wantHigh && finding.Severity != model.SeverityHigh {
				t.Fatalf("severity = %q, want high", finding.Severity)
			}
		})
	}

	if finding, err := DetectTLSSecretIssue(
		&corev1.Secret{}, now, time.Hour, 3,
	); err != nil || finding != nil {
		t.Fatalf("empty secret result = %#v, %v", finding, err)
	}
	if _, err := DetectTLSSecretIssue(
		&corev1.Secret{Data: map[string][]byte{
			"tls.crt": pem.EncodeToMemory(&pem.Block{
				Type: "CERTIFICATE", Bytes: []byte("bad"),
			}),
		}}, now, time.Hour, 3,
	); err == nil {
		t.Fatal("invalid certificate did not return an error")
	}
}

func TestEndpointReadinessTreatsUnavailableStates(t *testing.T) {
	falseValue := false
	trueValue := true
	cases := []struct {
		name     string
		endpoint discoveryv1.Endpoint
		want     bool
	}{
		{name: "unknown is usable", want: true},
		{
			name: "not ready",
			endpoint: discoveryv1.Endpoint{
				Conditions: discoveryv1.EndpointConditions{Ready: &falseValue},
			},
		},
		{
			name: "not serving",
			endpoint: discoveryv1.Endpoint{
				Conditions: discoveryv1.EndpointConditions{Serving: &falseValue},
			},
		},
		{
			name: "terminating",
			endpoint: discoveryv1.Endpoint{
				Conditions: discoveryv1.EndpointConditions{
					Terminating: &trueValue,
				},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := endpointCanReceiveTraffic(test.endpoint); got != test.want {
				t.Fatalf("usable = %v, want %v", got, test.want)
			}
		})
	}
}

func certificatePEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example"},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(
		rand.Reader, template, template, &key.PublicKey, key,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
