package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// runConfigRows blame a changed or missing Secret or ConfigMap for a
// failed Job: a run reads them when it starts, so no pod need be in
// view. The edit may be hours older than the run (see changeFrom).
var runConfigRows = []Row{
	{
		Name: "run-secret-missing-or-changed",
		Cause: Side{Kind: kube.KindSecret, Modes: []detection.Mode{
			ModeMissing, ModeChanged}},
		Link: LinkUses, Effect: Side{Kind: kube.KindJob}, Prior: 0.7,
	},
	{
		Name: "run-configmap-missing-or-changed",
		Cause: Side{Kind: kube.KindConfigMap, Modes: []detection.Mode{
			ModeMissing, ModeChanged}},
		Link: LinkUses, Effect: Side{Kind: kube.KindJob}, Prior: 0.7,
	},
}
