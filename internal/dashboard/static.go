package dashboard

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// web holds the dashboard UI: plain HTML, CSS and JavaScript with no
// build step, and the brand fonts. Nothing is loaded from elsewhere.
//
//go:embed web
var web embed.FS

// assetCache lets browsers keep a hashed asset for a year: its name
// changes whenever its content does.
const assetCache = "public, max-age=31536000, immutable"

// assetRef is how a file names another embedded file:
// {{asset:fonts/geist-mono.woff2}}. It becomes the hashed URL.
var assetRef = regexp.MustCompile(`\{\{asset:([a-z0-9./-]+)\}\}`)

// assetTypes are the content types of what web holds.
var assetTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
}

// shellRoutes are the UI's own paths; each serves the HTML shell.
var shellRoutes = []string{"/", "/signin", "/resolved", "/status"}

// asset is one file ready to serve.
type asset struct {
	body        []byte
	contentType string
}

// assets serves the UI: the shell at its routes, hashed files under
// /assets/.
type assets struct {
	shell  []byte
	byPath map[string]asset
}

// mustAssets builds the assets from the embedded files. A missing or
// broken file is a build error, so it panics.
func mustAssets() *assets {
	a, err := buildAssets(web)
	if err != nil {
		panic("dashboard assets: " + err.Error())
	}
	return a
}

// buildAssets hashes every file and rewrites the references between
// them. Leaves (fonts, images, scripts) go first, then stylesheets,
// which name fonts, then the shell, which names everything.
func buildAssets(root fs.FS) (*assets, error) {
	a := &assets{byPath: map[string]asset{}}
	urls := map[string]string{}
	for _, pass := range []func(string) bool{isLeaf, isStyle} {
		if err := a.addPass(root, urls, pass); err != nil {
			return nil, err
		}
	}
	shell, err := fs.ReadFile(root, "web/index.html")
	if err != nil {
		return nil, err
	}
	a.shell, err = rewriteRefs(shell, urls)
	return a, err
}

func isLeaf(name string) bool  { return !isStyle(name) }
func isStyle(name string) bool { return path.Ext(name) == ".css" }

func (a *assets) addPass(
	root fs.FS, urls map[string]string, want func(string) bool,
) error {
	return fs.WalkDir(root, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "web/index.html" || !want(p) {
			return err
		}
		ext := path.Ext(p)
		contentType, ok := assetTypes[ext]
		if !ok {
			return nil
		}
		body, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		if body, err = rewriteRefs(body, urls); err != nil {
			return err
		}
		name := strings.TrimPrefix(p, "web/")
		sum := sha256.Sum256(body)
		url := "/assets/" + strings.TrimSuffix(name, ext) + "." +
			hex.EncodeToString(sum[:])[:12] + ext
		urls[name] = url
		a.byPath[url] = asset{body: body, contentType: contentType}
		return nil
	})
}

// rewriteRefs replaces {{asset:name}} with the hashed URL of name. A
// reference to a file that has no URL yet is an error.
func rewriteRefs(body []byte, urls map[string]string) ([]byte, error) {
	var missing string
	out := assetRef.ReplaceAllFunc(body, func(m []byte) []byte {
		name := string(assetRef.FindSubmatch(m)[1])
		url, ok := urls[name]
		if !ok {
			missing = name
			return m
		}
		return []byte(url)
	})
	if missing != "" {
		return nil, &fs.PathError{Op: "reference", Path: missing,
			Err: fs.ErrNotExist}
	}
	return out, nil
}

// ServeHTTP serves the shell, an asset or 404.
func (a *assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if isShellRoute(r.URL.Path) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(a.shell)
		return
	}
	file, ok := a.byPath[r.URL.Path]
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", file.contentType)
	w.Header().Set("Cache-Control", assetCache)
	_, _ = w.Write(file.body)
}

func isShellRoute(p string) bool {
	for _, route := range shellRoutes {
		if p == route {
			return true
		}
	}
	id, ok := strings.CutPrefix(p, "/issues/")
	return ok && id != "" && !strings.Contains(id, "/")
}
