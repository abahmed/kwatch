package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerUsesStableOrderAndGETOnly(t *testing.T) {
	r := &Registry{}
	r.IncidentsCreate.Store(1)
	r.IncidentsUpdate.Store(2)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	r.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Index(body, `action="create"`) > strings.Index(body, `action="update"`) {
		t.Fatal("incident metrics are not in stable order")
	}
	post := httptest.NewRecorder()
	r.Handler().ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", post.Code)
	}
}
