package dashboard

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
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
	// The attempt is counted before the body is read, so many slow
	// requests at once cannot all pass the limit.
	attempt, reserved := a.reserve(r)
	if !reserved {
		a.tooMany(w, r)
		return
	}
	password, ok := readPassword(r)
	if !ok {
		attempt.forgive()
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validPassword(password) {
		a.noteProxyFailure(r)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	attempt.forgive()
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

// sessionCheck wraps sessionHandler for the sign-in page's question
// "am I signed in?". A request with no bearer token and no valid
// session cookie is not an attack, just a visitor: it gets 200
// {"signedIn": false} (so the browser logs no error), says nothing else
// and is not counted by the failure limiter. Any request that carries
// a bearer token, or a valid cookie, goes through the middleware as
// before, so a guessed token still counts and still gets 401.
func (a *Auth) sessionCheck(next http.Handler) http.Handler {
	checked := a.Middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, bearer := bearerToken(r); bearer || a.validCookie(r) {
			checked.ServeHTTP(w, r)
			return
		}
		if !onlyGet(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := "{\"signedIn\":false}\n"
		if c, err := r.Cookie(sessionCookie); err == nil &&
			a.sessions.fromEarlierStart(c.Value) {
			body = "{\"signedIn\":false,\"reason\":\"restarted\"}\n"
		}
		_, _ = w.Write([]byte(body))
	})
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

func (a *Auth) tooMany(w http.ResponseWriter, r *http.Request) {
	limiter, _ := a.limiterFor(r)
	secs := int(limiter.retryAfter().Seconds())
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

// limiterFor is the limit r counts against and its key: the client a
// trusted proxy names, else the connecting peer. Behind a trusted proxy
// only the named client is limited: the proxy appends the address it
// saw, so a client cannot rotate it, and one client's guesses never
// lock out the others. Without trusted proxies behind an Ingress, all
// clients share the one peer limit, so one guesser can lock everyone
// out for a minute: set dashboard.trustedProxies there.
func (a *Auth) limiterFor(r *http.Request) (*rateLimiter, string) {
	if client := a.forwardedClient(r); client != "" {
		return a.clientRate, client
	}
	return a.failRate, remoteHost(r)
}

// allowed reports whether r's client is under its failure limit.
func (a *Auth) allowed(r *http.Request) bool {
	limiter, key := a.limiterFor(r)
	return limiter.allowed(key)
}

// reserve counts one attempt for r's client; false past the limit.
func (a *Auth) reserve(r *http.Request) (attempt, bool) {
	limiter, key := a.limiterFor(r)
	return limiter.reserve(key)
}

// noteProxyFailure counts a failure that came through a trusted proxy
// against the proxy and logs once a minute when there are many: many
// clients behind it are guessing. It never refuses a request.
func (a *Auth) noteProxyFailure(r *http.Request) {
	if a.forwardedClient(r) == "" {
		return
	}
	peer := remoteHost(r)
	if a.proxyRate.record(peer) == maxProxyFailures {
		klog.InfoS("many dashboard sign-in failures through one proxy",
			"component", "dashboard-server", "proxy", peer,
			"failures", maxProxyFailures, "window", "1m")
	}
}

// loopback reports whether r came straight from this host, such as a
// port-forward. A trusted proxy on loopback is not a port-forward.
func (a *Auth) loopback(r *http.Request) bool {
	ip, err := netip.ParseAddr(remoteHost(r))
	return err == nil && ip.IsLoopback() && !a.peerTrusted(r)
}
