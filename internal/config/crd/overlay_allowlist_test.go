package crd

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/config"
)

func overlaySpec(spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": spec,
	}}
}

func TestOverlayRefusesSettingsOutsideTheAllowlist(t *testing.T) {
	for name, spec := range map[string]map[string]interface{}{
		"license.endpoint": {"license": map[string]interface{}{
			"endpoint": "http://evil.example"}},
		"license.key": {"license": map[string]interface{}{
			"key": "KW1-x.y"}},
		"healthCheck.statusEndpoint": {"healthCheck": map[string]interface{}{
			"statusEndpoint": true}},
		"watch.secrets": {"watch": map[string]interface{}{
			"secrets": true}},
		"app.hideProHints": {"app": map[string]interface{}{
			"hideProHints": true}},
		"app.communityProvider": {"app": map[string]interface{}{
			"communityProvider": "x"}},
		"kubelet.insecureSkipVerify": {"kubelet": map[string]interface{}{
			"insecureSkipVerify": true}},
	} {
		t.Run(name, func(t *testing.T) {
			err := ApplyOverlay(config.DefaultConfig(), overlaySpec(spec))
			if err == nil || !errorsIs(err, ErrInvalidOverlay) {
				t.Fatalf("err = %v, want ErrInvalidOverlay", err)
			}
		})
	}
}

func TestOverlayAllowsTheCRDSchemaSettings(t *testing.T) {
	spec := map[string]interface{}{
		"namespaces":  []interface{}{"apps"},
		"app":         map[string]interface{}{"clusterName": "c"},
		"maintenance": map[string]interface{}{"enabled": true},
		"upgrader":    map[string]interface{}{"disableUpdateCheck": true},
		"silences": []interface{}{map[string]interface{}{
			"namespaces": []interface{}{"x"}}},
		"workers": float64(2), // unknown: warned about, not refused
	}
	if err := ApplyOverlay(config.DefaultConfig(),
		overlaySpec(spec)); err != nil {
		t.Fatalf("a settable spec was refused: %v", err)
	}
}
