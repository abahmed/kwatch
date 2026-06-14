package handler

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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

	now := time.Now()
	for _, secret := range secrets {
		h.checkTLSSecret(secret, now, warnWindow)
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
	remaining := time.Until(expiry)
	cn := cert.Subject.CommonName

	if remaining < 0 {
		ev := h.eventWithConfig(event.Event{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TLSCertExpiringSoon",
			Logs:      "",
			Labels:    secret.Labels,
			Severity:  "high",
			Hint:      fmt.Sprintf("expired %v ago; CN=%s", (-remaining).Round(time.Hour), cn),
		})
		inc, action := h.correlator.Process(ev, key, nil)
		if action != model.ActionSkip {
			h.alertManager.NotifyIncident(inc, action)
		}
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
		ev := h.eventWithConfig(event.Event{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TLSCertExpiringSoon",
			Logs:      "",
			Labels:    secret.Labels,
			Severity:  severity,
			Hint:      fmt.Sprintf("expires in %dd (%s); CN=%s", daysLeft, expiry.Format("2006-01-02"), cn),
		})
		inc, action := h.correlator.Process(ev, key, nil)
		if action != model.ActionSkip {
			h.alertManager.NotifyIncident(inc, action)
		}
	} else {
		h.correlator.ResolveByResource("secret", key)
	}
}
