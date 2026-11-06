package dashboard

import (
	"net/http"
	"strings"
)

// notFoundPage is what a browser shows for an address the dashboard
// does not have: the shell's stylesheet and brand by their hashed
// asset links (buildAssets rewrites them), no script and no inline
// style, so the Content Security Policy holds, and a way back. Its
// title reads like every other page's: "Not found - kwatch".
const notFoundPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Not found - kwatch</title>
<link rel="icon" type="image/svg+xml" href="{{asset:favicon.svg}}">
<link rel="stylesheet" href="{{asset:tokens.css}}">
<link rel="stylesheet" href="{{asset:app.css}}">
</head>
<body>
<main class="center">
<div class="signin">
<a class="brand" href="/" aria-label="kwatch, overview">` +
	`<img class="mark" src="{{asset:favicon.svg}}" width="26" height="26" ` +
	`alt=""><span class="wordmark">kwatch</span></a>
<h1>Page not found</h1>
<p class="lead">The dashboard has no page at this address.</p>
<p><a class="ulink" href="/">Go to the Overview</a></p>
</div>
</main>
</body>
</html>
`

// serveNotFound answers 404: the not-found page for an address a
// person typed or followed, and the JSON error for a missing asset,
// which a page asked for, not a person.
func (a *assets) serveNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write(a.notFound)
	}
}
