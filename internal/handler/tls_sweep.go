package handler

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func (h *handler) SweepTLSSecrets() {
	if h.secretLister == nil {
		return
	}
	threshold := h.config.TlsMonitor.Threshold
	if threshold <= 0 {
		threshold = 30
	}
	warnWindow := time.Duration(threshold) * 24 * time.Hour

	secrets, err := h.secretLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "tls sweep: failed to list secrets from cache")
		return
	}

	for _, secret := range secrets {
		h.checkTLSSecret(secret, h.now(), warnWindow)
	}
}

func (h *handler) checkTLSSecret(secret *corev1.Secret, now time.Time, warnWindow time.Duration) {
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
		klog.ErrorS(err, "tls sweep: parse certificate", "secret", secret.Name, "namespace", secret.Namespace)
		return
	}

	key := secret.Namespace + "/" + secret.Name
	expiry := cert.NotAfter
	remaining := expiry.Sub(now)
	cn := cert.Subject.CommonName

	if remaining < 0 {
		h.signalEvent(&event.Signal{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TLSCertExpired",
			Owner:     key,
			Labels:    secret.Labels,
			Severity:  "high",
			Hint:      fmt.Sprintf("expired %v ago; CN=%s", (-remaining).Round(time.Hour), cn),
		})
	} else if remaining < warnWindow {
		daysLeft := int(remaining.Hours() / 24)
		severity := "normal"
		critical := h.config.TlsMonitor.CriticalThreshold
		if critical <= 0 {
			critical = 3
		}
		if daysLeft <= critical {
			severity = "high"
		}
		h.signalEvent(&event.Signal{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TLSCertExpiringSoon",
			Owner:     key,
			Labels:    secret.Labels,
			Severity:  severity,
			Hint:      fmt.Sprintf("expires in %dd (%s); CN=%s", daysLeft, expiry.Format("2006-01-02"), cn),
		})
	} else {
		h.correlator.ResolveByResource("secret", key)
	}
}
