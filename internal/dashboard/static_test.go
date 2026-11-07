package dashboard

import (
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// allowedURLs are the only absolute URLs the UI may contain. Neither is
// fetched: one is the SVG namespace, the other is the installer command
// the sign-in page shows as text to copy.
var allowedURLs = map[string]bool{
	"http://www.w3.org/2000/svg":   true,
	"https://kwatch.dev/kwatch.sh": true,
}

var absoluteURL = regexp.MustCompile(
	`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}[^\s"'<>)]*`)

func TestStaticAssetsContainNoExternalURLs(t *testing.T) {
	err := fs.WalkDir(web, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".woff2") ||
			strings.HasSuffix(p, ".txt") {
			return err
		}
		body, err := fs.ReadFile(web, p)
		if err != nil {
			return err
		}
		for _, m := range absoluteURL.FindAllString(string(body), -1) {
			if !allowedURLs[m] {
				t.Errorf("%s names an outside URL: %s", p, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticShellHasNoInlineScriptOrStyle(t *testing.T) {
	shell := string(mustAssets().shell)
	inline := []*regexp.Regexp{
		regexp.MustCompile(`<script(\s[^>]*)?>[^<]+</script>`),
		regexp.MustCompile(`<script(?:\s+(?:defer|type="[^"]*"))*\s*>`),
		regexp.MustCompile(`<style`),
		regexp.MustCompile(`\sstyle=`),
		regexp.MustCompile(`\son[a-z]+=`),
	}
	for _, re := range inline {
		if m := re.FindString(shell); m != "" {
			t.Errorf("shell has inline code the CSP would block: %q", m)
		}
	}
	if strings.Contains(shell, "{{asset:") {
		t.Error("shell has an unresolved asset reference")
	}
	for _, js := range []string{"look", "core", "rows", "searchq",
		"highlight", "listquery", "prefs", "visits", "dropdown",
		"selectmenu", "quickviews", "colmenu", "columns", "issuetable",
		"filterchips", "issuesfilters", "pager", "issueslive", "viewopts",
		"place", "drawer", "issues", "evidence", "timeline", "context",
		"objects", "insight", "share", "detail", "incpanel", "incnav",
		"trends", "api", "activity", "status", "refresh",
		"main", "keys"} {
		if !regexp.MustCompile(`src="/assets/js/` + js +
			`\.[0-9a-f]{12}\.js"`).MatchString(shell) {
			t.Errorf("shell does not load the hashed %s.js", js)
		}
	}
}

// The text size and contrast must apply before the page draws: look.js
// is the first script, loaded without defer, before any stylesheet.
func TestStaticShellAppliesTheLookBeforeTheFirstPaint(t *testing.T) {
	shell := string(mustAssets().shell)
	look := regexp.MustCompile(
		`<script src="/assets/js/look\.[0-9a-f]{12}\.js"></script>`).
		FindStringIndex(shell)
	if look == nil {
		t.Fatal("look.js is not loaded as a blocking script")
	}
	for _, first := range []string{"<script", "<link rel=\"stylesheet\""} {
		if at := strings.Index(shell, first); at < look[0] {
			t.Errorf("%s comes before look.js", first)
		}
	}
}

func TestStaticShellRoutesAndHashedAssets(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/", "/signin", "/issues",
		"/activity", "/activity?since=24h", "/kwatch",
		"/issues?status=resolved", "/issues/inc-1"} {
		w := call(t, s, http.MethodGet, path, "")
		if w.Code != 200 ||
			!strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") ||
			w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %v", path, w.Code, w.Header())
		}
	}
	for _, path := range []string{"/nope", "/issues/", "/issues/a/b",
		"/assets/app.css"} {
		if w := call(t, s, http.MethodGet, path, ""); w.Code != 404 {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
	if w := call(t, s, http.MethodPost, "/", ""); w.Code != 405 {
		t.Errorf("POST /: %d, want 405", w.Code)
	}
	css := regexp.MustCompile(`/assets/app\.[0-9a-f]{12}\.css`).
		FindString(string(mustAssets().shell))
	w := call(t, s, http.MethodGet, css, "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != assetCache ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("%s: %d %v", css, w.Code, w.Header())
	}
	font := regexp.MustCompile(`/assets/fonts/geist-mono\.[0-9a-f]{12}\.woff2`).
		FindString(w.Body.String())
	if fw := call(t, s, http.MethodGet, font, ""); fw.Code != 200 ||
		fw.Header().Get("Content-Type") != "font/woff2" {
		t.Errorf("font %q: %d", font, fw.Code)
	}
}

func TestStaticOldAddressesRedirectToTheNewOnes(t *testing.T) {
	s := newTestServer()
	cases := map[string]string{
		"/status":                     "/",
		"/insights":                   "/",
		"/resolved":                   "/issues?status=resolved",
		"/resolved?since=24h&q=a+b":   "/issues?status=resolved&q=a+b&since=24h",
		"/?severity=page&age=today":   "/issues?severity=page&age=today",
		"/?q=ns%3Ashop&group=off":     "/issues?q=ns%3Ashop&group=off",
		"/?acked=false":               "/issues?acked=false",
		"/resolved?status=active&x=1": "/issues?status=resolved&x=1",
	}
	for path, want := range cases {
		w := call(t, s, http.MethodGet, path, "")
		if w.Code != http.StatusFound || w.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want %q", path, w.Code,
				w.Header().Get("Location"), want)
		}
	}
	if w := call(t, s, http.MethodGet, "/?utm=x", ""); w.Code != 200 {
		t.Errorf("/ without list filters: %d, want the Overview", w.Code)
	}
}

func TestStaticEveryResponseCarriesTheCSP(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/", "/api/status", "/nope"} {
		w := call(t, s, http.MethodGet, path, testToken)
		csp := w.Header().Get("Content-Security-Policy")
		for _, part := range []string{"default-src 'none'",
			"script-src 'self'", "style-src 'self'", "connect-src 'self'",
			"frame-ancestors 'none'", "base-uri 'none'",
			"worker-src 'self'", "manifest-src 'self'"} {
			if !strings.Contains(csp, part) {
				t.Errorf("%s: CSP %q lacks %q", path, csp, part)
			}
		}
		if strings.Contains(csp, "unsafe-inline") {
			t.Errorf("%s: CSP allows inline code", path)
		}
	}
}

func TestStaticBuildRefusesAMissingReference(t *testing.T) {
	root := fstest.MapFS{
		"web/index.html": {Data: []byte(`<link href="{{asset:gone.css}}">`)},
	}
	if _, err := buildAssets(root); err == nil {
		t.Error("a reference to a missing file built")
	}
	root["web/gone.css"] = &fstest.MapFile{Data: []byte("body{}")}
	if _, err := buildAssets(root); err == nil {
		t.Error("built without what the not-found page names")
	}
	root["web/app.css"] = &fstest.MapFile{Data: []byte("body{}")}
	root["web/tokens.css"] = &fstest.MapFile{Data: []byte(":root{}")}
	root["web/favicon.svg"] = &fstest.MapFile{Data: []byte("<svg/>")}
	if _, err := buildAssets(root); err == nil {
		t.Error("built without the app manifest")
	}
	root["web/manifest.webmanifest"] = &fstest.MapFile{Data: []byte("{}")}
	if _, err := buildAssets(root); err != nil {
		t.Errorf("complete set: %v", err)
	}
}

// An address the dashboard does not have shows a small HTML page with
// a way back to the Overview; a missing asset and an unknown API path
// keep the JSON error.
func TestStaticUnknownAddressShowsANotFoundPage(t *testing.T) {
	s := newTestServer()
	w := call(t, s, http.MethodGet, "/nope", "")
	if w.Code != 404 ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(w.Body.String(), `href="/">Go to the Overview`) {
		t.Errorf("/nope: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	for _, path := range []string{"/assets/nope.css", "/api/nope"} {
		w := get(t, s, path)
		if w.Code != 404 || !strings.HasPrefix(
			w.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s: %d %v", path, w.Code, w.Header())
		}
	}
}

// The not-found page looks like the rest of the dashboard: the title
// format of every page, the shell's stylesheet and brand by their
// hashed links, and nothing inline the Content Security Policy blocks.
func TestStaticNotFoundPageUsesTheSharedLook(t *testing.T) {
	s := newTestServer()
	page := call(t, s, http.MethodGet, "/nope", "").Body.String()
	css := regexp.MustCompile(`/assets/app\.[0-9a-f]{12}\.css`).
		FindString(string(s.assets.shell))
	for _, want := range []string{"<title>Not found - kwatch</title>",
		`<link rel="stylesheet" href="` + css + `">`,
		`class="wordmark">kwatch</span>`, "Page not found"} {
		if css == "" || !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	for _, banned := range []string{"{{asset:", "<style", "style=",
		"<script"} {
		if strings.Contains(page, banned) {
			t.Errorf("page has %q", banned)
		}
	}
	if w := call(t, s, http.MethodGet, css, ""); w.Code != 200 {
		t.Errorf("its stylesheet: %d", w.Code)
	}
	reply := string(s.assets.badRequest)
	if !strings.HasPrefix(reply, "HTTP/1.1 400 Bad Request\r\n") ||
		!strings.HasSuffix(reply, page) {
		t.Errorf("400 reply = %q", reply)
	}
}
