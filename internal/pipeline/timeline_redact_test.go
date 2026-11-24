package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// An application writes event messages, so the timeline must not keep a
// secret it put there.
func TestEventEntryRedactsMessage(t *testing.T) {
	id := inventory.EntityID{Kind: "Pod", Namespace: "prod", Name: "web"}
	note := inventory.Note{
		Reason: "Failed",
		Message: "auth Authorization: Bearer abcdef0123456789abcdef01 " +
			"for ops@example.com via https://deploy:s3cretpw@git.example.com/r",
	}
	got := eventEntry(id, note, timelineStart).Text
	assert.NotContains(t, got, "abcdef0123456789abcdef01")
	assert.NotContains(t, got, "ops@example.com")
	assert.NotContains(t, got, "s3cretpw")
	assert.Contains(t, got, "git.example.com")
}
