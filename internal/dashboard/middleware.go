package dashboard

import (
	"net"
	"net/http"

	"k8s.io/klog/v2"
)

// contentSecurityPolicy allows only same-origin scripts, styles, fonts,
// images and requests: no inline script or style, no framing.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; " +
	"style-src 'self'; img-src 'self' data:; font-src 'self'; " +
	"connect-src 'self'; form-action 'self'; base-uri 'none'; " +
	"frame-ancestors 'none'"

// securityHeaders sets the headers every dashboard response carries,
// errors included. Cache-Control is no-store; hashed assets replace it.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
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

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// accessLog logs one line per request: remote host, method, path and
// status. It never logs headers, so the token never reaches the log.
// An open page polls every few seconds, so a successful read is logged
// only at verbosity 2; sign-in, sign-out and every failure (401, 403,
// 429, 5xx) stay at the default level, where an operator needs them.
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
