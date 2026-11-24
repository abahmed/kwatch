//go:build browser

package dashboard

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/rootcause"
)

// browserGistCause is a cause with a long one-line summary, which the
// Issues table shows under the title.
func browserGistCause() *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Summary: "The api pods cannot reach " +
		"their database: the db address in api-config stopped " +
		"resolving after the cluster DNS change, so every new " +
		"connection fails and the pods restart again and again"}
}

// A row with a likely cause shows it as a second line under the title,
// cut to two lines on a desktop and one on a phone.
func TestBrowserIssuesCauseSecondLine(t *testing.T) {
	srv := listsServer(t, nil)
	for _, w := range []struct {
		px    int64
		lines string
	}{{1280, "2"}, {390, "1"}} {
		text, done := polishPage(t, srv.URL, "/issues",
			".it-gist", w.px)
		got := text(`(() => {
		  const a = document.querySelector(
		    '#issues-table a[href="/issues/inc-003"]');
		  const g = a.closest("tr").querySelector(".it-gist");
		  const cs = getComputedStyle(g);
		  const lines = Math.round(g.clientHeight /
		    parseFloat(cs.lineHeight));
		  return g.textContent + "|" + lines + "|" +
		    (g.scrollHeight > g.clientHeight + 1);
		})()`)
		want := "|" + w.lines + "|true"
		if !strings.HasPrefix(got, "The api pods cannot reach") ||
			!strings.HasSuffix(got, want) {
			t.Errorf("%dpx: %q, want ...%s", w.px, got, want)
		}
		done()
	}
}
