package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultClaimPending is how long a claim may wait for a volume. Dynamic
// provisioning normally binds within seconds.
const DefaultClaimPending = 2 * time.Minute

// Claim detects PersistentVolumeClaims that cannot bind, were lost, or
// cannot finish a resize.
type Claim struct{}

// Name implements signal.Detector.
func (Claim) Name() string { return "claim" }

// Kinds implements signal.Detector.
func (Claim) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindPVC} }

// Detect implements signal.Detector.
func (Claim) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	var out []signal.Signal
	switch text(e, kube.AttrPhase) {
	case "Pending":
		since := valueSince(e, kube.AttrPhase)
		if sustained(ctx, since, DefaultClaimPending) {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonPersistentVolumeClaim,
				Severity: signal.Warning, Since: since,
				Summary: "Volume claim has been waiting for a volume for " +
					format.Duration(ctx.Now.Sub(since)),
			})
		}
	case "Lost":
		out = append(out, signal.Signal{
			Reason:   constant.ReasonPersistentVolumeClaim,
			Severity: signal.Critical,
			Since:    valueSince(e, kube.AttrPhase),
			Summary:  "Volume claim lost its bound volume",
		})
	}
	for _, conditionType := range []string{
		"ControllerResizeError", "NodeResizeError",
	} {
		if status, reason, since := condition(e, conditionType); status ==
			"True" {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonPersistentVolumeClaim,
				Severity: signal.Warning, Since: since,
				Summary: "Volume resize failed (" + reason + ")",
				Evidence: []signal.Evidence{{
					Label: "message",
					Value: conditionMessage(e, conditionType),
				}},
			})
		}
	}
	return out
}

// Volume detects failed PersistentVolumes.
type Volume struct{}

// Name implements signal.Detector.
func (Volume) Name() string { return "volume" }

// Kinds implements signal.Detector.
func (Volume) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindPV} }

// Detect implements signal.Detector.
func (Volume) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	if text(e, kube.AttrPhase) != "Failed" {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonPersistentVolume, Severity: signal.Critical,
		Since:   valueSince(e, kube.AttrPhase),
		Summary: "Persistent volume failed (" + text(e, kube.AttrReason) + ")",
		Evidence: []signal.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}}
}
