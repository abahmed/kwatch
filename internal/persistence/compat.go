package persistence

import (
	"time"

	kubernetes "k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
)

// NewManager preserves the historical function-clock constructor for
// standalone and embedded callers. Application composition uses
// NewManagerWithClock.
func NewManager(
	client kubernetes.Interface,
	namespace string,
	clocks ...func() time.Time,
) *Manager {
	return NewManagerWithClock(
		client, namespace, clock.Func(clock.From(clocks)),
	)
}
