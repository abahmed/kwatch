package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNewHandler(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}

func TestNewHandlerWithMonitors(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 60},
		OomMonitor:        config.OomMonitor{Enabled: true, Threshold: 3, WindowMinutes: 10},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}

func TestNewHandlerPendingPodMonitorZeroThreshold(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 0},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}
