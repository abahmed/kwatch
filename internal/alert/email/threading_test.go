package email

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

var headerLine = regexp.MustCompile(`(?m)^([A-Za-z-]+): (.*)$`)

// threadHeaders sends msg to a fake relay and returns the headers it got.
func threadHeaders(
	t *testing.T, msg notification.Message,
) map[string]string {
	t.Helper()
	f := &fakeSMTP{}
	port := f.start(t)
	c := NewEmail(relayConfig(port, "kwatch@Example.test",
		"ops@example.test"), "dev", transport.Dependencies{})
	require.NotNil(t, c)
	require.NoError(t, c.SendIncident(t.Context(), msg))
	f.mu.Lock()
	defer f.mu.Unlock()
	head, _, _ := strings.Cut(f.data, "\n\n")
	out := map[string]string{}
	for _, m := range headerLine.FindAllStringSubmatch(head, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// Every incident mail carries a Message-ID, and the updates and the
// resolve name the opening's, so a mail client threads them under it.
func TestEmailThreadsAnIncidentUnderItsOpening(t *testing.T) {
	opening := threadHeaders(t, providertest.Announce())
	update := threadHeaders(t, providertest.Update())
	resolve := threadHeaders(t, providertest.Resolve())

	root := opening["Message-ID"]
	assert.Regexp(t, `^<kwatch\.[0-9a-f]{24}\.1@example\.test>$`, root)
	assert.Empty(t, opening["In-Reply-To"])
	assert.Empty(t, opening["References"])
	for name, headers := range map[string]map[string]string{
		"update": update, "resolve": resolve,
	} {
		assert.Equal(t, root, headers["In-Reply-To"], name)
		assert.Equal(t, root, headers["References"], name)
		assert.NotEmpty(t, headers["Message-ID"], name)
		assert.NotEqual(t, root, headers["Message-ID"], name)
	}
	assert.NotEqual(t, update["Message-ID"], resolve["Message-ID"])
}

// A retried mail keeps its ID; another incident, and the same incident
// in another cluster, get their own.
func TestEmailMessageIDIsStablePerMail(t *testing.T) {
	msg := providertest.Announce()
	first, _ := threadIDs(msg, "dev", "example.test")
	again, _ := threadIDs(msg, "dev", "example.test")
	assert.Equal(t, first, again)

	elsewhere, _ := threadIDs(msg, "prod", "example.test")
	assert.NotEqual(t, first, elsewhere)
	other := msg
	other.Key = "another-incident"
	otherID, _ := threadIDs(other, "dev", "example.test")
	assert.NotEqual(t, first, otherID)

	// Two mails of one revision must not share an ID either.
	a, b := providertest.Update(), providertest.Update()
	b.Short = "something else"
	idA, _ := threadIDs(a, "dev", "example.test")
	idB, _ := threadIDs(b, "dev", "example.test")
	assert.NotEqual(t, idA, idB)
}

// Notices and summaries are not conversations: they get no fixed ID,
// which a mail server could take for one mail delivered twice.
func TestEmailNoticesAndSummariesGetNoMessageID(t *testing.T) {
	for _, msg := range []notification.Message{
		notification.Notice("kwatch started"), providertest.Summary(),
	} {
		id, parent := threadIDs(msg, "dev", "example.test")
		assert.Empty(t, id)
		assert.Empty(t, parent)
	}
	assert.Equal(t, "kwatch.invalid", idDomain("no-domain"))
	assert.Equal(t, "example.test", idDomain("bot@Example.test"))
}
