package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The fix line is an action with a verb and the value to move past,
// never a bare field path.
func TestFixTextSaysWhatToDo(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	svc := inventory.CoreID(kube.KindService, "shop", "api")
	cases := []struct {
		place detection.Place
		want  string
	}{
		{detection.Place{Object: api, Container: "app",
			Path: "resources.limits.cpu", Value: "200m"},
			"Fix: raise the CPU limit of container app in deployment/api " +
				"above 200m."},
		{detection.Place{Object: api, Container: "app",
			Path: "readinessProbe.httpGet.port", Value: "8081"},
			"Fix: set the readiness probe of container app in " +
				"deployment/api to a port the app listens on (now 8081)."},
		{detection.Place{Object: api, Container: "app",
			Path: "livenessProbe", Value: "periodSeconds 10"},
			"Fix: give the liveness probe of container app in " +
				"deployment/api more time (now periodSeconds 10)."},
		{detection.Place{Object: api, Container: "app",
			Path:  "env[KEY].valueFrom.secretKeyRef",
			Value: "secret stripe"},
			"Fix: correct the Secret that env var KEY reads of container " +
				"app in deployment/api (now secret stripe)."},
		{detection.Place{Object: api, Path: "spec.replicas", Value: "0"},
			"Fix: scale up deployment/api (now 0 replicas)."},
		{detection.Place{Object: api, Path: "spec.replicas", Value: "1"},
			"Fix: scale up deployment/api (now 1 replica)."},
		{detection.Place{Object: svc, Path: "spec.ports",
			Value: "80/->web"},
			"Fix: correct the ports of service/api (now port 80 → web)."},
		{detection.Place{Object: svc, Path: "spec.ports",
			Value: "443/TCP->https,53/UDP->dns"},
			"Fix: correct the ports of service/api (now ports " +
				"443 → https, 53/UDP → dns)."},
		{detection.Place{Object: api,
			Path: "spec.template.spec.tolerations", Value: "none"},
			"Fix: add the toleration the node taint needs to " +
				"deployment/api."},
		{detection.Place{Object: svc, Path: "spec.selector",
			Value: "app=ap"},
			"Fix: correct the selector of service/api so it matches " +
				"the pods (now app=ap)."},
		{detection.Place{Object: svc, Path: "spec.unknownField",
			Value: "x"},
			"Fix: change spec.unknownField of service/api (now x)."},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, fixText(tc.place, false), tc.place.Path)
	}
}

// An image the lead already named is not repeated in the fix line; a
// short value such as a replica count always stays.
func TestFixTextLeavesOutAnImageTheNoteNamed(t *testing.T) {
	image := "registry.example.com/payments:2.3"
	assert.True(t, valueNamed("the new version (payments:2.3) fails",
		image))
	assert.False(t, valueNamed("a maximum of 6 replicas", "6"))
	place := detection.Place{Object: inventory.CoreID(kube.KindDeployment,
		"shop", "payments"), Container: "app", Path: "image", Value: image}
	assert.Equal(t, "Fix: change the image of container app in "+
		"deployment/payments.", fixText(place, true))
}
