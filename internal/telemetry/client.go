package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// Endpoint is the first-party adoption telemetry endpoint.
const Endpoint = "https://api.kwatch.dev/v1/telemetry/heartbeat"

// WeeklyInterval is the minimum interval between anonymous heartbeats.
const WeeklyInterval = 7 * 24 * time.Hour

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

type payload struct {
	ClusterUUID string `json:"cluster_uuid"`
	Version     string `json:"kwatch_version"`
}

// ShouldSend reports whether the weekly heartbeat is due.
func ShouldSend(lastSent, now time.Time) bool {
	return lastSent.IsZero() || now.Sub(lastSent) >= WeeklyInterval
}

// Report sends one anonymous heartbeat. It returns nil only after receiving a
// successful 2xx response; callers should persist the send time only then.
func Report(ctx context.Context, client *http.Client, endpoint, clusterID, version string) error {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	if endpoint == "" || !uuidPattern.MatchString(clusterID) || !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid telemetry identity")
	}
	body, err := json.Marshal(payload{ClusterUUID: clusterID, Version: version})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kwatch-telemetry/1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telemetry endpoint returned status %d", resp.StatusCode)
	}
	return nil
}
