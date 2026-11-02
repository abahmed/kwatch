package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// decodeBadQuery reads a 400 body.
func decodeBadQuery(t *testing.T, w *httptest.ResponseRecorder) queryErrorBody {
	t.Helper()
	var out queryErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return out
}

func TestWriteBadQueryCutsALongTerm(t *testing.T) {
	w := httptest.NewRecorder()
	long := strings.Repeat("x", 500)
	writeBadQuery(w, badQuery("node", long, "Not a node name: %q.", long))
	got := decodeBadQuery(t, w)
	if w.Code != http.StatusBadRequest || got.Param != "node" ||
		len([]rune(got.Term)) != maxErrorTerm ||
		len(got.Error) > 2*maxErrorTerm+32 {
		t.Errorf("%d %+v", w.Code, got)
	}
}

func TestWriteBadQueryNamesTheParamWhenNoTermIsWrong(t *testing.T) {
	w := httptest.NewRecorder()
	writeBadQuery(w, badQuery("q", "", "The search is too long."))
	if got := decodeBadQuery(t, w); got.Term != "q" || got.Param != "q" {
		t.Errorf("body = %+v", got)
	}
}

func TestListRejectsTooManyFailureTypes(t *testing.T) {
	path := "/api/incidents?"
	for i := range maxModes + 1 {
		path += "mode=M" + strings.Repeat("a", i+1) + "&"
	}
	w := get(t, filterFixture(), path)
	got := decodeBadQuery(t, w)
	if w.Code != http.StatusBadRequest || got.Param != "mode" ||
		got.Error != "Too many failure types: 20 is the limit." {
		t.Errorf("%d %s", w.Code, w.Body.String())
	}
}
