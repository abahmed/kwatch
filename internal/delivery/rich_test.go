package delivery

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestPrepareIncidentKeepsBlocksAndAddsFallbackNotice(t *testing.T) {
	m := prepareIncident(providertest.Rich(), nil, "slack", 0)
	if len(m.Doc) != 5 || !strings.Contains(m.Plain(),
		"(Sent through the fallback because slack failed.)") {
		t.Fatalf("blocks = %d, plain:\n%s", len(m.Doc), m.Plain())
	}
	if !strings.HasSuffix(m.Note, "slack failed.)") {
		t.Fatalf("the Note keeps the notice too: %q", m.Note)
	}
}

func TestPrepareIncidentFlattensWhenCutToProviderLimit(t *testing.T) {
	m := prepareIncident(providertest.Rich(), nil, "", 40)
	if m.Doc != nil || len(m.NoteText()) > 40 {
		t.Fatalf("a cut message is plain lines: %+v", m)
	}
}

func TestCombinedResolveAppendsTheOpeningBlocks(t *testing.T) {
	resolve := providertest.Rich()
	opening := providertest.Rich()
	resolve.Opening = &opening
	got := combinedResolve(resolve)
	if len(got.Doc) != 4+1+4 {
		t.Fatalf("blocks = %d", len(got.Doc))
	}
}
