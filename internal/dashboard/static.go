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
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
}

// rootFiles are served at the root by a fixed name, never hashed, by
// their content type: the app manifest and the service worker, which
// browsers find by that name (a worker's scope is the folder it is
// served from, so /sw.js covers the whole dashboard). What they name
// ({{asset:…}}) is rewritten to the hashed address.
var rootFiles = map[string]string{
	"manifest.webmanifest": "application/manifest+json",
	"sw.js":                "text/javascript; charset=utf-8",
}

// shellRoutes are the UI's own pages (Overview, Issues, Activity,
// kwatch); each serves the HTML shell.
var shellRoutes = []string{"/", "/signin", "/issues", "/activity",
	"/kwatch"}

// asset is one file ready to serve.
type asset struct {
	body        []byte
	contentType string
}

// assets serves the UI: the shell at its routes, hashed files under
// /assets/, and the not-found page (static_not_found.go) as the 404
// and, through pageListener, as net/http's own 400.
type assets struct {
	shell      []byte
	root       map[string][]byte
	notFound   []byte
	badRequest []byte
	byPath     map[string]asset
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
	a := &assets{byPath: map[string]asset{}, root: map[string][]byte{}}
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
	for name := range rootFiles {
		body, err := fs.ReadFile(root, "web/"+name)
		if err != nil {
			return nil, err
		}
		if a.root[name], err = rewriteRefs(body, urls); err != nil {
			return nil, err
		}
	}
	if a.shell, err = rewriteRefs(shell, urls); err != nil {
		return nil, err
	}
	a.shell = withDocsRef(a.shell)
	if a.notFound, err = rewriteRefs([]byte(notFoundPage), urls); err != nil {
		return nil, err
	}
	a.badRequest = badRequestReply(a.notFound, false)
	return a, nil
}

func isLeaf(name string) bool { return !isStyle(name) }
func isRootFile(p string) bool {
	_, ok := rootFiles[strings.TrimPrefix(p, "web/")]
	return ok
}
func isStyle(name string) bool { return path.Ext(name) == ".css" }

func (a *assets) addPass(
	root fs.FS, urls map[string]string, want func(string) bool,
) error {
	return fs.WalkDir(root, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "web/index.html" ||
			isRootFile(p) || !want(p) {
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
	if redirectOld(w, r) {
		return
	}
	if body, ok := a.root[strings.TrimPrefix(r.URL.Path, "/")]; ok {
		// Never kept long: browsers check these by their fixed URL.
		name := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", rootFiles[name])
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(body)
		return
	}
	if isShellRoute(r.URL.Path) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(a.shell)
		return
	}
	file, ok := a.byPath[r.URL.Path]
	if !ok {
		a.serveNotFound(w, r)
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
