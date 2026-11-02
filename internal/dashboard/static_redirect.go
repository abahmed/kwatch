package dashboard

import (
	"net/http"
	"net/url"
)

// listParams are the query keys of the old Active issues list at "/".
// A link to "/" carrying one of them is a list link and moves to
// /issues with the same query.
var listParams = []string{"q", "severity", "namespace", "state", "age",
	"since", "sort", "dir", "group", "kind", "status", "offset", "limit",
	"page", "pageSize"}

// oldRoute is where an address of the old two-list layout lives now:
// "/status" (the Overview) is "/", "/resolved?…" is
// "/issues?status=resolved&…", and "/" with list filters is
// "/issues?…". It returns "" for every other address.
func oldRoute(u *url.URL) string {
	switch u.Path {
	case "/status":
		return "/"
	case "/resolved":
		q := u.Query()
		q.Del("status")
		rest := q.Encode()
		if rest != "" {
			rest = "&" + rest
		}
		return "/issues?status=resolved" + rest
	case "/":
		q := u.Query()
		for _, k := range listParams {
			if q.Has(k) {
				return "/issues?" + u.RawQuery
			}
		}
	}
	return ""
}

// redirectOld answers an old address with a redirect to the new one,
// so bookmarks and pasted links keep working; false when it is not
// an old address.
func redirectOld(w http.ResponseWriter, r *http.Request) bool {
	to := oldRoute(r.URL)
	if to == "" {
		return false
	}
	http.Redirect(w, r, to, http.StatusFound)
	return true
}
