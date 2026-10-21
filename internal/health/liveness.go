package health

import (
	"net/http"

	"k8s.io/klog/v2"
)

// livenessCheck reports why the process is no longer alive; nil when it
// is. The application sets it while a session runs work that must
// progress, such as the pipeline loop.
type livenessCheck func() error

// SetLivenessCheck makes /healthz fail while check returns an error, so
// the kubelet restarts a process whose loop hung. Nil removes it.
func (h *HealthServer) SetLivenessCheck(check func() error) {
	if check == nil {
		h.liveness.Store(nil)
		return
	}
	c := livenessCheck(check)
	h.liveness.Store(&c)
}

// livenessError runs the liveness check, if any.
func (h *HealthServer) livenessError() error {
	check := h.liveness.Load()
	if check == nil {
		return nil
	}
	return (*check)()
}

func writeNotAlive(w http.ResponseWriter, err error) {
	klog.ErrorS(err, "health: liveness check failed")
	w.WriteHeader(http.StatusServiceUnavailable)
	if _, werr := w.Write([]byte(err.Error())); werr != nil {
		klog.ErrorS(werr, "health: write healthz response")
	}
}
