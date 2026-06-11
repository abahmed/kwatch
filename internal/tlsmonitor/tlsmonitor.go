package tlsmonitor

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type TlsMonitor struct {
	client       kubernetes.Interface
	config       *config.TlsMonitor
	alertManager *alert.AlertManager
	correlator   *correlation.Engine
}

func New(
	client kubernetes.Interface,
	cfg *config.TlsMonitor,
	alertManager *alert.AlertManager,
	correlator *correlation.Engine,
) *TlsMonitor {
	return &TlsMonitor{
		client:       client,
		config:       cfg,
		alertManager: alertManager,
		correlator:   correlator,
	}
}

func (m *TlsMonitor) Start(ctx context.Context) {
	if !m.config.Enabled {
		return
	}

	m.checkCerts()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("tls monitor stopped")
			return
		case <-ticker.C:
			m.checkCerts()
		}
	}
}

func (m *TlsMonitor) checkCerts() {
	threshold := m.config.Threshold
	if threshold <= 0 {
		threshold = 30
	}
	warnWindow := time.Duration(threshold) * 24 * time.Hour

	secrets, err := m.client.CoreV1().Secrets("").List(context.Background(), metav1.ListOptions{
		FieldSelector: "type=kubernetes.io/tls",
	})
	if err != nil {
		klog.ErrorS(err, "tls monitor: failed to list secrets")
		return
	}

	now := time.Now()
	for _, secret := range secrets.Items {
		m.checkSecret(secret, now, warnWindow)
	}
}

func (m *TlsMonitor) checkSecret(secret corev1.Secret, now time.Time, warnWindow time.Duration) {
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
		klog.V(4).InfoS("tls monitor: failed to parse certificate", "secret", secret.Name, "namespace", secret.Namespace, "error", err)
		return
	}

	expiry := cert.NotAfter
	remaining := time.Until(expiry)

	if remaining < 0 {
		ev := event.Event{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TlsCertExpired",
			Logs:      "",
			Labels:    secret.Labels,
			Hint:      fmt.Sprintf("TLS certificate expired %v ago", -remaining.Round(time.Hour)),
		}
		inc, action := m.correlator.Process(ev, secret.Namespace+"/"+secret.Name, nil)
		if action != model.ActionSkip {
			m.alertManager.NotifyIncident(inc, action)
		}
	} else if remaining < warnWindow {
		days := int(remaining.Hours() / 24)
		ev := event.Event{
			Resource:  "secret",
			PodName:   secret.Name,
			Namespace: secret.Namespace,
			Reason:    "TlsCertExpiringSoon",
			Logs:      "",
			Labels:    secret.Labels,
			Hint:      fmt.Sprintf("TLS certificate expires in %d days (%s)", days, expiry.Format(time.RFC3339)),
		}
		inc, action := m.correlator.Process(ev, secret.Namespace+"/"+secret.Name, nil)
		if action != model.ActionSkip {
			m.alertManager.NotifyIncident(inc, action)
		}
	} else {
		m.correlator.ResolveByResource("secret", secret.Namespace+"/"+secret.Name)
	}
}
