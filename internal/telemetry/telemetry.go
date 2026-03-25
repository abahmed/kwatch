package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/sirupsen/logrus"
)

type Telemetry struct {
	enabled bool
	apiURL  string
}

func NewTelemetry(cfg *config.Telemetry) *Telemetry {
	return &Telemetry{
		enabled: cfg.Enabled,
		apiURL:  constant.TelemetryAPIURL,
	}
}

func NewTelemetryWithURL(cfg *config.Telemetry, apiURL string) *Telemetry {
	return &Telemetry{
		enabled: cfg.Enabled,
		apiURL:  apiURL,
	}
}

type EventPayload struct {
	ClusterID string `json:"cluster_id"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

func (t *Telemetry) SendEvent(ctx context.Context, clusterID, version string) error {
	if !t.enabled {
		logrus.Debug("telemetry is disabled, skipping event")
		return nil
	}

	payload := EventPayload{
		ClusterID: clusterID,
		Version:   version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, t.apiURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logrus.Warnf("telemetry request returned status: %d", resp.StatusCode)
		return nil
	}

	logrus.Debugf("telemetry event sent successfully for cluster: %s", clusterID)
	return nil
}
