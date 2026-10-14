package opsgenie

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Opsgenie Alert API requests for the
// canonical messages: create, the later-revision updates and the close.
func TestContract(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"secret"}, LimitBytes: descriptionLimit,
		KeyOf: aliasOf, Check: checkAlertShape,
	})
}

// aliasOf reads the alias from the create body or the alert path.
func aliasOf(t *testing.T, r providertest.Request) string {
	if r.Method == "POST" && r.Path == "/v2/alerts" {
		return r.JSON(t)["alias"].(string)
	}
	rest := strings.TrimPrefix(r.RawPath, "/v2/alerts/")
	alias, err := url.PathUnescape(strings.Split(rest, "/")[0])
	if err != nil || r.Query != "identifierType=alias" {
		t.Fatalf("not an alias path: %s?%s", r.RawPath, r.Query)
	}
	return alias
}

func checkAlertShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an alert")
		}
		return
	}
	for _, r := range reqs {
		if r.Header.Get("Authorization") != "GenieKey secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
	}
	if name == "resolve" {
		if len(reqs) != 1 || !strings.HasSuffix(reqs[0].Path, "/close") {
			t.Fatalf("resolve must only close the alias: %+v", reqs)
		}
		return
	}
	var p ogPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Message == "" || p.Alias == "" || p.Priority != "P1" {
		t.Fatalf("create lacks required fields: %+v", p)
	}
	if len(p.Message) > messageLimit || len(p.Description) >
		descriptionLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.Message),
			len(p.Description))
	}
	providertest.AssertFencesBalanced(t, p.Description)
	wantReqs := 1
	if name == "notify-update" {
		wantReqs = 4
	}
	if len(reqs) != wantReqs {
		t.Fatalf("%s sent %d requests, want %d", name, len(reqs), wantReqs)
	}
}
