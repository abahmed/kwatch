package security

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectTLSSecretIssue checks the first certificate in a TLS Secret. Parsing
// failures are returned so the caller can report them without creating an
// incident; missing certificate data is treated as an unrelated Secret.
func DetectTLSSecretIssue(
	secret *corev1.Secret,
	now time.Time,
	warnWindow time.Duration,
	criticalDays int,
) (*model.Observation, error) {
	if secret == nil || len(secret.Data["tls.crt"]) == 0 {
		return nil, nil
	}
	block, _ := pem.Decode(secret.Data["tls.crt"])
	if block == nil {
		return nil, nil
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	remaining := certificate.NotAfter.Sub(now)
	if remaining < 0 {
		return observe.Object(
			"secret", secret, constant.ReasonTLSCertExpired,
		).WithSeverity(model.SeverityHigh).WithHint(fmt.Sprintf(
			"expired %v ago; CN=%s",
			(-remaining).Round(time.Hour), certificate.Subject.CommonName,
		)), nil
	}
	if remaining >= warnWindow {
		return nil, nil
	}
	daysLeft := int(remaining.Hours() / 24)
	if criticalDays <= 0 {
		criticalDays = 3
	}
	severity := model.SeverityNormal
	if daysLeft <= criticalDays {
		severity = model.SeverityHigh
	}
	return observe.Object(
		"secret", secret, constant.ReasonTLSCertExpiringSoon,
	).WithSeverity(severity).WithHint(fmt.Sprintf(
		"expires in %dd (%s); CN=%s",
		daysLeft,
		certificate.NotAfter.Format("2006-01-02"),
		certificate.Subject.CommonName,
	)), nil
}
