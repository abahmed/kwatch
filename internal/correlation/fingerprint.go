package correlation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// StableFingerprint identifies the failure class rather than the current
// object instance. Pod names and UIDs are intentionally excluded so a
// replacement Pod continues the same incident and feedback history.
func StableFingerprint(ev event.Event, owner string, cs *model.ContainerState) string {
	resource := ev.Resource
	if resource == "" {
		resource = "pod"
	}
	container := ev.ContainerName
	if container == "." {
		container = ""
	}
	identity := owner
	if ev.Resource == "pod" && ev.OwnerKind == "" && (owner == "" || owner == ev.PodName) {
		if lineage := strings.TrimSpace(ev.PodLineageID); lineage != "" {
			identity = "lineage/" + stableIdentityHash(ev.Namespace+":"+lineage)
		} else if uid := strings.TrimSpace(ev.PodUID); uid != "" {
			identity = "uid/" + uid
		}
	}
	canonical := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(ev.Namespace)),
		strings.ToLower(strings.TrimSpace(resource)),
		strings.ToLower(strings.TrimSpace(ev.OwnerKind)),
		strings.ToLower(strings.TrimSpace(identity)),
		normalizeReason(ev.Reason),
		strings.ToLower(strings.TrimSpace(container)),
	}, "|")
	return fingerprintHash(canonical)
}

func legacyFingerprint(key model.IncidentKey) string {
	return fingerprintHash("legacy|" + string(key))
}

func fingerprintHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("fp-%s", hex.EncodeToString(sum[:8]))
}
