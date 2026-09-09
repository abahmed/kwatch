package handler

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (h *handler) SweepTLSSecrets() {
	if h.listers.Secret == nil {
		return
	}
	threshold := h.config.TlsMonitor.Threshold
	if threshold <= 0 {
		threshold = 30
	}
	warnWindow := time.Duration(threshold) * 24 * time.Hour

	secrets, err := h.listers.Secret.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "tls sweep: failed to list secrets from cache")
		return
	}

	for _, secret := range secrets {
		h.checkTLSSecret(secret, h.now(), warnWindow)
	}
}

func (h *handler) checkTLSSecret(
	secret *corev1.Secret,
	now time.Time,
	warnWindow time.Duration,
) {
	certData, ok := secret.Data["tls.crt"]
	if !ok || len(certData) == 0 {
		return
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		klog.ErrorS(
			err,
			"tls sweep: parse certificate",
			"secret",
			secret.Name,
			"namespace",
			secret.Namespace,
		)
		return
	}

	expiry := cert.NotAfter
	remaining := expiry.Sub(now)
	cn := cert.Subject.CommonName

	// A renewed certificate moves between these branches, so the finding is
	// reconciled rather than each branch resolving the other's reason.
	var current *model.Observation
	switch {
	case remaining < 0:
		current = observe.Object(
			"secret", secret, constant.ReasonTLSCertExpired,
		).WithSeverity(model.SeverityHigh).WithHint(fmt.Sprintf(
			"expired %v ago; CN=%s",
			(-remaining).Round(time.Hour),
			cn,
		))
	case remaining < warnWindow:
		daysLeft := int(remaining.Hours() / 24)
		severity := model.SeverityNormal
		critical := h.config.TlsMonitor.CriticalThreshold
		if critical <= 0 {
			critical = 3
		}
		if daysLeft <= critical {
			severity = model.SeverityHigh
		}
		current = observe.Object(
			"secret", secret, constant.ReasonTLSCertExpiringSoon,
		).WithSeverity(severity).WithHint(fmt.Sprintf(
			"expires in %dd (%s); CN=%s",
			daysLeft,
			expiry.Format("2006-01-02"),
			cn,
		))
	}
	h.reconcile(
		model.NewObjectRef("secret", secret.Namespace, secret.Name),
		findings(current),
	)
}
