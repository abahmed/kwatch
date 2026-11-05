package dashboard

import (
	"net"
	"net/http"

	"k8s.io/klog/v2"
)

// contentSecurityPolicy allows only same-origin scripts, styles, fonts,
// images, requests, the service worker and the app manifest: no inline
// script or style, no framing.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; " +
	"style-src 'self'; img-src 'self' data:; font-src 'self'; " +
	"connect-src 'self'; worker-src 'self'; manifest-src 'self'; " +
	"form-action 'self'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'"

// hstsValue tells browsers to use https for the dashboard's host for a
// year. It leaves subdomains alone: they are not kwatch's to decide.
const hstsValue = "max-age=31536000"

// pageHeaders are the headers every dashboard response carries, errors
// and the raw 400 of an unparsable request (badRequestReply) included.
var pageHeaders = [][2]string{
	{"X-Content-Type-Options", "nosniff"},
	{"X-Frame-Options", "DENY"},
	{"Cache-Control", "no-store"},
	{"Referrer-Policy", "no-referrer"},
	{"Content-Security-Policy", contentSecurityPolicy},
	{"Cross-Origin-Opener-Policy", "same-origin"},
	{"Cross-Origin-Resource-Policy", "same-origin"},
}

// securityHeaders sets pageHeaders on every dashboard response.
// Cache-Control is no-store; hashed assets replace it.
// Strict-Transport-Security is added when hsts says so.
func securityHeaders(
	next http.Handler, hsts func(*http.Request) bool,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if hsts(r) {
			h.Set("Strict-Transport-Security", hstsValue)
		}
		for _, kv := range pageHeaders {
			h.Set(kv[0], kv[1])
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the connection: live
// streams flush and set deadlines through it.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// accessLog logs one line per request: remote host, method, path and
// status. It never logs headers, so the token never reaches the log.
// An open page polls every few seconds, so a successful read is logged
// only at verbosity 2. A 401 or 404 is something anyone can cause at
// will, so it is logged at verbosity 1; sign-in, sign-out and the other
// failures (403, 429, 5xx) stay at the default level.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		verbosity := klog.Level(0)
		if r.Method == http.MethodGet && rec.status < http.StatusBadRequest {
			verbosity = 2
		}
		if rec.status == http.StatusUnauthorized ||
			rec.status == http.StatusNotFound {
			verbosity = max(verbosity, 1)
		}
		klog.V(verbosity).InfoS("dashboard request",
			"component", "dashboard-server",
			"remote", remoteHost(r), "method", r.Method,
			"path", r.URL.Path, "status", rec.status)
	})
}

// remoteHost is the client address without its port, so limits and logs
// follow the client rather than one connection.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
