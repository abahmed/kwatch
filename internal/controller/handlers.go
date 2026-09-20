package controller

import (
	"k8s.io/client-go/tools/cache"

	kwk8s "github.com/abahmed/kwatch/internal/k8s"
)

func safeEventHandler(
	resource string,
	handler cache.ResourceEventHandler,
) cache.ResourceEventHandler {
	return kwk8s.SafeEventHandler("controller", resource, handler)
}
