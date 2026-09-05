package crdwatch

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"gopkg.in/yaml.v3"

	"github.com/abahmed/kwatch/internal/config"
)

// ApplyStartupConfig overlays the optional KwatchConfig before components are
// constructed. A CRD is deliberately a startup-only source; live changes
// cause a restart through Watcher.
func ApplyStartupConfig(ctx context.Context, cfg *config.Config, restCfg *rest.Config, namespace string) error {
	if !cfg.CrdConfig.Enabled {
		return nil
	}
	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	list, err := dc.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(list.Items) == 0 {
		return nil
	}
	if len(list.Items) != 1 {
		return fmt.Errorf("expected at most one KwatchConfig in namespace %q", namespace)
	}
	spec, ok := list.Items[0].Object["spec"]
	if !ok {
		return nil
	}
	if err := rejectSecretConfig(spec); err != nil {
		return err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	config.ResetDerivedSilences(cfg)
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return err
	}
	return config.RebuildAfterOverlay(cfg)
}

func rejectSecretConfig(spec interface{}) error {
	root, ok := spec.(map[string]interface{})
	if !ok {
		return nil
	}
	if _, exists := root["alert"]; exists {
		return stderrors.New(
			"KwatchConfig.spec.alert is forbidden; mount provider " +
				"credentials through config.yaml file references",
		)
	}
	for _, path := range [][]string{
		{"healthCheck", "diagnosticsToken"},
		{"heartbeatMonitor", "url"},
	} {
		section, ok := root[path[0]].(map[string]interface{})
		if !ok {
			continue
		}
		if _, exists := section[path[1]]; exists {
			return fmt.Errorf(
				"KwatchConfig.spec.%s.%s is forbidden; configure it "+
					"through a mounted Secret",
				path[0],
				path[1],
			)
		}
	}
	return nil
}
