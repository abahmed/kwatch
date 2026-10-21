package announce

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func entries(n int) []auditEntry {
	var out []auditEntry
	for i := range n {
		out = append(out, auditEntry{id: fmt.Sprintf("inc-%03d", i),
			title: func() string { return "title" }})
	}
	return out
}

// Up to the bound every incident of the digest is worded; past it the
// audit entry says how many more there were and names each of them, so
// an incident carried by a digest can always be found in the log.
func TestDigestAuditItemsWordTheFirstAndNameTheRest(t *testing.T) {
	items := auditItems(entries(MaxAuditItems + 26))

	require.Len(t, items, MaxAuditItems+1)
	assert.Equal(t, "inc-000: title", items[0])
	last := items[MaxAuditItems]
	assert.True(t, strings.HasPrefix(last, "and 26 more: inc-100, "), last)
	assert.True(t, strings.HasSuffix(last, "inc-125"), last)
}

// A digest within the bound has no "and more" item.
func TestDigestAuditItemsAreAllWordedWithinTheBound(t *testing.T) {
	items := auditItems(entries(MaxAuditItems))

	assert.Len(t, items, MaxAuditItems)
	for _, item := range items {
		assert.NotContains(t, item, "more:")
	}
}

// Even the IDs of the hidden ones are bounded; the rest are counted.
func TestDigestAuditHiddenIDsAreBoundedAndCounted(t *testing.T) {
	rest := entries(MaxHiddenIDs + 7)

	text := hiddenItem(rest)

	assert.Contains(t, text, fmt.Sprintf("and %d more: ", MaxHiddenIDs+7))
	assert.Contains(t, text, "and 7 others")
	assert.NotContains(t, text, fmt.Sprintf("inc-%03d", MaxHiddenIDs))
}
