package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

// A leftover webhook is announced at notify, so the pagers hear it as a
// warning. When it starts refusing creates it rises to a page: that
// update raises the page under the page key, which a pager that keeps
// an alert's first urgency would otherwise never page for.
func TestNotifyThenPageRaisesUnderThePageKey(t *testing.T) {
	result := replayNamed(t, "webhook-idle-then-rejects")

	require.Len(t, result.Messages, 2)
	warn, page := result.Messages[0], result.Messages[1]
	assert.Equal(t, "warning", warn.Route.Severity)
	assert.False(t, warn.RaisesPage || warn.PageKeyed, "first alert")
	assert.Equal(t, "critical", page.Route.Severity)
	assert.True(t, page.RaisesPage, "the rise opens the page key")
	assert.True(t, page.PageKeyed)
	base := warn.AlertKey("dev")
	assert.Equal(t, base, page.AlertKey("dev"), "same incident")
	assert.Equal(t, base+notification.PageKeySuffix,
		page.AlertKeyVariant("dev", notification.PageKeySuffix, 255))
}
