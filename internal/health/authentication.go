package health

import (
	"crypto/subtle"
	"net/http"

	"k8s.io/klog/v2"
)

func (h *HealthServer) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.requireDiagnosticsAuth(w, r) {
			return
		}
		next(w, r)
	}
}

func (h *HealthServer) requireDiagnosticsAuth(
	w http.ResponseWriter,
	r *http.Request,
) bool {
	// Protected diagnostics never allow anonymous access. Liveness, readiness,
	// health, and metrics are registered separately and do not use this guard.
	if h.diagnosticsToken == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte(
			"diagnostics authentication is not configured",
		)); err != nil {
			klog.ErrorS(err, "health: write diagnostics configuration response")
		}
		return false
	}
	token := r.Header.Get("Authorization")
	if subtle.ConstantTimeCompare(
		[]byte(token), []byte("Bearer "+h.diagnosticsToken),
	) == 1 {
		return true
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusUnauthorized)
	if _, err := w.Write([]byte("unauthorized")); err != nil {
		klog.ErrorS(err, "health: write unauthorized response")
	}
	return false
}
