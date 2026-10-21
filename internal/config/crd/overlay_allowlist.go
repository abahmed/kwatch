package crd

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
)

// keyTree lists the keys a KwatchConfig may set at one level. A key whose
// tree is nil is settable with everything below it.
type keyTree map[string]keyTree

// overlayKeys is every setting a KwatchConfig may change: the fields of
// the CRD schema (deploy/crd.yaml). It is an allowlist, so a setting added
// to the configuration later is refused here until it is listed on
// purpose. The rest of the configuration stays in the mounted file, which
// needs more privilege to edit than a KwatchConfig: provider credentials,
// the license, outbound transport and TLS trust, file paths, the health
// server, Secret watching and the kubelet connection.
var overlayKeys = keyTree{
	"namespaces":              nil,
	"reasons":                 nil,
	"namespaceSelector":       nil,
	"ignoreContainerNames":    nil,
	"ignorePodNames":          nil,
	"ignoreContainerMessages": nil,
	"ignoreNodeReasons":       nil,
	"ignoreNodeMessages":      nil,
	"silences":                nil,
	"resyncSeconds":           nil,
	"severityByOwnerKind":     nil,
	"severityByReason":        nil,
	"activeProbeMonitor":      nil,
	"templates":               nil,
	"runbooks":                nil,
	"clusterCheckins":         nil,
	"crd":                     {"enabled": nil},
	"telemetry":               {"enabled": nil},
	"auditLog":                {"enabled": nil},
	"upgrader":                nil,
	"heartbeatMonitor":        {"enabled": nil, "interval": nil},
	"maintenance": {
		"enabled": nil, "annotation": nil, "untilAnnotation": nil,
	},
	"app": {
		"clusterName": nil, "disableStartupMessage": nil,
		"logFormatter": nil,
	},
}

// rejectForbiddenSettings refuses a spec that sets a configuration key a
// KwatchConfig may not change. A key kwatch does not know at all is a typo
// and is only warned about (config.NoteOverlayUnknownKeys).
func rejectForbiddenSettings(spec interface{}) error {
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
	return checkKeys(overlayKeys, root, nil)
}

func checkKeys(
	allowed keyTree, node map[string]interface{}, path []string,
) error {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		here := append(append([]string(nil), path...), key)
		sub, ok := allowed[key]
		switch {
		case !ok && config.KnownKey(here...):
			return fmt.Errorf(
				"KwatchConfig.spec.%s is forbidden; configure it in the "+
					"mounted configuration",
				strings.Join(firstLeaf(here, node[key]), "."))
		case !ok || len(sub) == 0:
			continue
		}
		child, isMap := node[key].(map[string]interface{})
		if !isMap {
			continue
		}
		if err := checkKeys(sub, child, here); err != nil {
			return err
		}
	}
	return nil
}

// firstLeaf extends path down the first key of each nested section, so the
// error names the setting ("healthCheck.port"), not just its section.
func firstLeaf(path []string, value interface{}) []string {
	section, ok := value.(map[string]interface{})
	if !ok || len(section) == 0 {
		return path
	}
	keys := make([]string, 0, len(section))
	for key := range section {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	next := append(append([]string(nil), path...), keys[0])
	if !config.KnownKey(next...) {
		return path
	}
	return firstLeaf(next, section[keys[0]])
}
