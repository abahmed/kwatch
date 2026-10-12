package email

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func emailConfigForTLS() map[string]interface{} {
	return map[string]interface{}{
		"from": "kwatch@example.test", "to": "ops@example.test",
		"host": "smtp.example.test", "port": "587",
	}
}

func TestEmailInheritsApplicationTLSSettings(t *testing.T) {
	pool := x509.NewCertPool()
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: pool, InsecureSkipVerify: true, // #nosec G402
		},
	}}
	c := NewEmail(emailConfigForTLS(), "dev",
		transport.Dependencies{HTTPClient: client})
	require.NotNil(t, c)

	got := c.smtpConfig.clientTLS()
	require.Same(t, pool, got.RootCAs)
	require.True(t, got.InsecureSkipVerify)
	require.Equal(t, "smtp.example.test", got.ServerName)
}

func TestEmailTLSDefaultsWithoutApplicationClient(t *testing.T) {
	c := NewEmail(emailConfigForTLS(), "dev", transport.Dependencies{})
	require.NotNil(t, c)

	got := c.smtpConfig.clientTLS()
	require.Nil(t, got.RootCAs)
	require.False(t, got.InsecureSkipVerify)
	require.Equal(t, "smtp.example.test", got.ServerName)
}

func TestEmailTLSConfigIsNotShared(t *testing.T) {
	base := &tls.Config{}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: base}}
	c := NewEmail(emailConfigForTLS(), "dev",
		transport.Dependencies{HTTPClient: client})
	require.NotNil(t, c)

	c.smtpConfig.clientTLS().ServerName = "mutated"
	require.Empty(t, base.ServerName)
}
