package pvcmonitor

import (
	"time"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"k8s.io/client-go/kubernetes"
)

type PvcMonitor struct {
	client       kubernetes.Interface
	config       *config.PvcMonitor
	alertManager *alertmanager.AlertManager
	notifiedPvc  map[string]bool
}

// NewPvcMonitor returns new instance of pvc monitor
func NewPvcMonitor(
	client kubernetes.Interface,
	config *config.PvcMonitor,
	alertManager *alertmanager.AlertManager) *PvcMonitor {
	return &PvcMonitor{
		client:       client,
		config:       config,
		alertManager: alertManager,
		notifiedPvc:  make(map[string]bool),
	}
}

func (p *PvcMonitor) Start() {
	if !p.config.Enabled {
		return
	}

	// check at startup
	p.checkUsage()

	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.checkUsage()
	}
}
