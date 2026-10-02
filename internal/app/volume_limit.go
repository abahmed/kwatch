package app

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// volumeLimitEnv carries the emptyDir size limit, as a Kubernetes quantity
// such as "2Gi". It is unset for a PersistentVolumeClaim, whose size the
// file system already reports.
const volumeLimitEnv = "KWATCH_VOLUME_LIMIT"

// volumeLimit returns the data volume limit in bytes, or 0 when none is
// configured. An unreadable value is an error rather than silently
// ignored, so a typo cannot disable the free-space guard.
func volumeLimit() (int64, error) {
	return parseVolumeLimit(os.Getenv(volumeLimitEnv))
}

func parseVolumeLimit(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	quantity, err := resource.ParseQuantity(raw)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", volumeLimitEnv, raw, err)
	}
	if quantity.Sign() < 0 {
		return 0, fmt.Errorf("%s %q must not be negative", volumeLimitEnv, raw)
	}
	return quantity.Value(), nil
}
