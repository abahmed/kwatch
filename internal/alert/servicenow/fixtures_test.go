package servicenow

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// testPassword is built from pieces so secret scanners do not take the
// fixture for a real credential.
var testPassword = strings.Join([]string{"fixture", "pass", "9f2c"}, "-")

var testNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// record is one row of the fake incident table.
type record map[string]string

// fakeInstance is an in-process ServiceNow Table API: it keeps incidents
// in memory, answers correlation id queries and records every request.
type fakeInstance struct {
	t      *testing.T
	server *httptest.Server

	mu       sync.Mutex
	rows     map[string]record
	order    []string
	requests []string
	tokens   int
	// fail, when set, answers a request instead of the table.
	fail func(w http.ResponseWriter, r *http.Request) bool
}

func newFakeInstance(t *testing.T) *fakeInstance {
	t.Helper()
	f := &fakeInstance{t: t, rows: map[string]record{}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeInstance) dependencies() transport.Dependencies {
	return transport.Dependencies{
		HTTPClient: f.server.Client(),
		Clock:      clock.Func(func() time.Time { return testNow }),
	}
}

// provider builds a provider against the fake with basic auth and the
// given extra settings.
func (f *fakeInstance) provider(extra map[string]interface{}) *ServiceNow {
	f.t.Helper()
	config := map[string]interface{}{
		"instanceURL": f.server.URL,
		"user":        "kwatch", "password": testPassword,
	}
	for k, v := range extra {
		config[k] = v
	}
	p := NewServiceNow(config, "prod", f.dependencies())
	if p == nil {
		f.t.Fatal("provider refused the test settings")
	}
	return p
}

func (f *fakeInstance) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	fail := f.fail
	f.mu.Unlock()
	if fail != nil && fail(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == tokenPath {
		f.grantToken(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		f.query(w, r.URL.Query().Get("sysparm_query"))
	case http.MethodPost:
		f.insert(w, body)
	case http.MethodPatch:
		f.update(w, r.URL.Path, body)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeInstance) grantToken(w http.ResponseWriter) {
	f.mu.Lock()
	f.tokens++
	n := f.tokens
	f.mu.Unlock()
	_, _ = fmt.Fprintf(w,
		`{"access_token":"tok-%d","expires_in":1800}`, n)
}

func (f *fakeInstance) insert(w http.ResponseWriter, body []byte) {
	var row record
	if err := json.Unmarshal(body, &row); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := fmt.Sprintf("sys%04d", len(f.order)+1)
	row["state"] = "1"
	f.rows[id] = row
	f.order = append(f.order, id)
	f.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"result":{"sys_id":%q}}`, id)
}

func (f *fakeInstance) update(w http.ResponseWriter, path string, body []byte) {
	id := path[strings.LastIndex(path, "/")+1:]
	var fields record
	_ = json.Unmarshal(body, &fields)
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"No Record found"}}`))
		return
	}
	for k, v := range fields {
		if k == "work_notes" || k == "comments" {
			row[k] = strings.TrimPrefix(row[k]+"\n"+v, "\n")
			continue
		}
		row[k] = v
	}
	_, _ = fmt.Fprintf(w, `{"result":{"sys_id":%q}}`, id)
}

// query answers correlation_id=X^active=true^state!=S like the Table API.
func (f *fakeInstance) query(w http.ResponseWriter, q string) {
	want := map[string]string{}
	notState := ""
	for _, term := range strings.Split(q, "^") {
		if k, v, ok := strings.Cut(term, "!="); ok && k == "state" {
			notState = v
			continue
		}
		k, v, _ := strings.Cut(term, "=")
		want[k] = v
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var hits []string
	for _, id := range f.order {
		row := f.rows[id]
		active := row["state"] != "7" && row["state"] != "8"
		if row["correlation_id"] == want["correlation_id"] && active &&
			row["state"] != notState {
			hits = append(hits, fmt.Sprintf(`{"sys_id":%q}`, id))
		}
	}
	_, _ = fmt.Fprintf(w, `{"result":[%s]}`, strings.Join(hits, ","))
}

func (f *fakeInstance) row(id string) record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := record{}
	for k, v := range f.rows[id] {
		out[k] = v
	}
	return out
}

func (f *fakeInstance) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.order)
}

func (f *fakeInstance) tokenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokens
}

func (f *fakeInstance) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeInstance) setFail(
	fail func(http.ResponseWriter, *http.Request) bool,
) {
	f.mu.Lock()
	f.fail = fail
	f.mu.Unlock()
}
