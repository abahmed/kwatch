package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/model"
)

func TestNotificationFingerprintTracksMissingServicePort(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{Resource: "service", Name: "api"},
		Evidence: model.Evidence{Facts: model.Facts{
			MissingServicePortKind:  "target port",
			MissingServicePortValue: "TCP/8080",
		}},
	}
	first := notificationFingerprint(inc)
	inc.Facts.MissingServicePortValue = "TCP/9090"
	assert.NotEqual(t, first, notificationFingerprint(inc))
}

func TestNotificationFingerprintTracksServiceBackendChanges(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{Resource: "service", Name: "api"},
		Evidence: model.Evidence{Facts: model.Facts{
			BackendsObserved: true, BackendPods: 3,
			UnreadyBackendPods: 3,
		}},
	}
	first := notificationFingerprint(inc)
	inc.Facts.SharedFailingNode = "node-a"
	assert.NotEqual(t, first, notificationFingerprint(inc))
	first = notificationFingerprint(inc)
	inc.Facts.BackendPods = 2
	assert.NotEqual(t, first, notificationFingerprint(inc))
}
