package servicenow

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const tablePath = "/api/now/table/incident"

var recordRef = regexp.MustCompile(`/incident/([A-Za-z0-9]+)$`)

// TestContract pins the exact Table API requests: the correlation id
// lookup, the create, the work notes and the resolve. The recorder is
// served over TLS because the provider only talks https.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"result":{"sys_id":"abc123"}}`))
		}
	}
	tlsServer := httptest.NewUnstartedServer(rec.Server.Config.Handler)
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)
	deps := rec.Dependencies()
	deps.HTTPClient = tlsServer.Client()
	p := NewServiceNow(map[string]interface{}{
		"instanceURL": tlsServer.URL,
		"user":        "kwatch", "password": testPassword,
	}, "dev", deps)
	providertest.RunContract(t, providertest.Contract{
		Send: p.SendIncident, Rec: rec,
		Headers:    []string{"Authorization", "Accept"},
		Secrets:    []string{testPassword},
		LimitBytes: bodyLimit,
		KeyOf:      recordKey,
		Check:      checkRecordShape,
	})
}

// recordKey is the record a request addresses; a create addresses the
// one the recorder answers with.
func recordKey(t *testing.T, r providertest.Request) string {
	if m := recordRef.FindStringSubmatch(r.Path); m != nil {
		return m[1]
	}
	if r.Method == http.MethodPost {
		return "abc123"
	}
	t.Fatalf("request addresses no record: %s %s", r.Method, r.Path)
	return ""
}

func checkRecordShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	for _, r := range reqs {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") ||
			!strings.HasPrefix(r.Path, tablePath) {
			t.Fatalf("%s: %s %s", name, r.Method, r.Path)
		}
		if r.Method == http.MethodGet {
			continue
		}
		for field, value := range r.JSON(t) {
			text, _ := value.(string)
			if utf8.RuneCountInString(text) > bodyLimit {
				t.Fatalf("%s: %s holds %d characters", name, field,
					utf8.RuneCountInString(text))
			}
		}
	}
	if name == "digest" && len(reqs) != 0 {
		t.Fatalf("a digest reached the instance")
	}
}
