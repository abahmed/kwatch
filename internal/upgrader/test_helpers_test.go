package upgrader

import (
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/persistence"
)

func newTestPersistenceManager(
	client kubernetes.Interface,
	namespace string,
) *persistence.Manager {
	return persistence.NewManagerWithClock(
		client, namespace, clock.RealClock{},
	)
}
