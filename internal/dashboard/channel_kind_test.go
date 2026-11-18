package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

// The dashboard calls a provider a pager exactly when the shared list
// says it pages a person, the list the Community ranking reads too.
func TestChannelKindPagerIsTheSharedPagingList(t *testing.T) {
	for _, name := range providercatalog.Names() {
		got := channelKind(name) == channelPager
		assert.Equal(t, providercatalog.PagesPerson(name), got, name)
	}
}

// Every listed kind is one the catalog knows, so a renamed provider
// cannot silently fall back to "other".
func TestChannelKindsNameKnownProviders(t *testing.T) {
	known := map[string]bool{}
	for _, name := range providercatalog.Names() {
		known[name] = true
	}
	for name := range channelKinds {
		assert.True(t, known[name], name)
	}
}
