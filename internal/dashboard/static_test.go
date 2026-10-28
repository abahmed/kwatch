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
	for _, js := range []string{"core", "lists", "detail", "status", "main"} {
		if !regexp.MustCompile(`src="/assets/js/` + js +
			`\.[0-9a-f]{12}\.js"`).MatchString(shell) {
			t.Errorf("shell does not load the hashed %s.js", js)
		}
	}
}

func TestStaticShellRoutesAndHashedAssets(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/", "/signin", "/resolved", "/status",
		"/issues/inc-1"} {
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

func TestStaticEveryResponseCarriesTheCSP(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/", "/api/status", "/nope"} {
		w := call(t, s, http.MethodGet, path, testToken)
		csp := w.Header().Get("Content-Security-Policy")
		for _, part := range []string{"default-src 'none'",
			"script-src 'self'", "style-src 'self'", "connect-src 'self'",
			"frame-ancestors 'none'", "base-uri 'none'"} {
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
	if _, err := buildAssets(root); err != nil {
		t.Errorf("complete set: %v", err)
	}
}
