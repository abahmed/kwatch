package app

import (
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/rbac"
)

func configureSecurityMonitor(
	runtime config.RuntimeConfig,
	client kubernetes.Interface,
	now func() time.Time,
) *rbac.Monitor {
	return rbac.NewWithRuntimeConfig(client, runtime, clock.Func(now))
}
