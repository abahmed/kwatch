package dashboard

import (
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxLoginBody bounds the sign-in request body.
const maxLoginBody = 4 << 10

// loginHandler serves POST /login with {"password": "..."}: 204 and a
// session cookie, 401 for a wrong password, 429 when the client failed
// too often, 403 https_required for plain HTTP in shared mode.
func (a *Auth) loginHandler(w http.ResponseWriter, r *http.Request) {
	if !a.formChecks(w, r) {
		return
	}
	client := a.clientKey(r)
	if !a.failRate.allowed(client) {
		a.tooMany(w)
		return
	}
	password, ok := readPassword(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validPassword(password) {
		a.failRate.record(client)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	value, expires, err := a.sessions.issue()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, a.cookie(value, int(sessionLifetime.Seconds()),
		expires.UTC().Format(http.TimeFormat)))
	w.WriteHeader(http.StatusNoContent)
}

// logoutHandler serves POST /logout: the session is signed out and the
// cookie cleared. It always answers 204.
func (a *Auth) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if !a.formChecks(w, r) {
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.sessions.revoke(c.Value)
	}
	http.SetCookie(w, a.cookie("", -1, ""))
	w.WriteHeader(http.StatusNoContent)
}

// sessionHandler answers GET /api/session behind the auth middleware:
// 204 means signed in.
func sessionHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireHTTPS answers 403 https_required in shared mode when the
// request did not come over https, so the browser can tell the person
// to use the https address. In local mode it lets everything through.
func (a *Auth) requireHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.shared && !a.isHTTPS(r) {
			writeError(w, http.StatusForbidden, "https_required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// formChecks applies what both sign-in and sign-out need: POST only,
// https in shared mode and a same-origin browser request.
func (a *Auth) formChecks(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	if a.shared && !a.isHTTPS(r) {
		writeError(w, http.StatusForbidden, "https_required")
		return false
	}
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (a *Auth) tooMany(w http.ResponseWriter) {
	secs := int(a.failRate.retryAfter().Seconds())
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "too many requests")
}

// cookie builds the session cookie: HttpOnly, SameSite=Strict, Path=/,
// no Domain, and Secure in shared mode only (decision D2: Safari drops a
// Secure cookie on http://localhost).
func (a *Auth) cookie(value string, maxAge int, expires string) *http.Cookie {
	c := &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.shared,
		SameSite: http.SameSiteStrictMode,
	}
	if expires != "" {
		c.RawExpires = expires
	}
	return c
}

// readPassword reads {"password": "..."} from a JSON body. Requiring
// the JSON content type keeps cross-site form posts out.
func readPassword(r *http.Request) (string, bool) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return "", false
	}
	var body struct {
		Password string `json:"password"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxLoginBody))
	if err := dec.Decode(&body); err != nil {
		return "", false
	}
	return body.Password, true
}

// sameOrigin accepts a request without an Origin header (not a browser
// form) or one whose Origin host is the request's own host.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

// isHTTPS reports whether the browser reached kwatch over https: kwatch
// served TLS itself, or a trusted proxy that terminated TLS said so in
// X-Forwarded-Proto. The header counts only with trustProxy and never
// from loopback, so a port-forward can never claim https.
func (a *Auth) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !a.trustProxy || loopback(r) {
		return false
	}
	return strings.EqualFold(lastHeaderValue(r, "X-Forwarded-Proto"),
		"https")
}

// clientKey is who failures are counted against. Behind a trusted
// proxy every request comes from the proxy, so the client is the last
// X-Forwarded-For hop, the one the proxy added.
func (a *Auth) clientKey(r *http.Request) string {
	if a.trustProxy && !loopback(r) {
		if hop := lastHeaderValue(r, "X-Forwarded-For"); hop != "" {
			return hop
		}
	}
	return remoteHost(r)
}

func loopback(r *http.Request) bool {
	ip := net.ParseIP(remoteHost(r))
	return ip != nil && ip.IsLoopback()
}

// lastHeaderValue is the last comma-separated value of a header that may
// repeat, trimmed.
func lastHeaderValue(r *http.Request, name string) string {
	values := r.Header.Values(name)
	if len(values) == 0 {
		return ""
	}
	parts := strings.Split(values[len(values)-1], ",")
	return strings.TrimSpace(parts[len(parts)-1])
}
