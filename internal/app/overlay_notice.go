package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/notification"
)

// stateOverlayRejected is the state key that holds the identity of the
// refused KwatchConfig the operator was last told about.
const stateOverlayRejected = "config.overlay.rejected"

// overlayNoticeText tells the providers that the KwatchConfig was refused
// at start and what kwatch runs with instead. It carries no detail of the
// refusal: the log has it, and a notice is no place to quote settings.
func overlayNoticeText(o overlayOutcome) string {
	text := notification.MarkerNotify + " kwatch could not apply the " +
		"KwatchConfig and "
	if o.lastGood {
		text += "runs with the last one that worked. "
	} else {
		text += "runs with the mounted configuration only, so the " +
			"silences and filters set in it are not active. "
	}
	return text + "Fix or delete the KwatchConfig; the kwatch log says " +
		"why it was refused."
}

// announceOverlay sends the notice once per refused spec. The record
// comes first, so a failed write never repeats the message; a KwatchConfig
// that applies cleanly again clears it, so a later refusal is told again.
func announceOverlay(deps *serverDeps, disk diskState) {
	var send func(string)
	if deps.deliveryManager != nil {
		send = deps.deliveryManager.Notify
	}
	noticeOverlay(disk, deps.overlay, send)
}

// noticeOverlay is announceOverlay with the sender passed in.
func noticeOverlay(disk diskState, o overlayOutcome, send func(string)) {
	var told string
	if _, err := disk.get(stateOverlayRejected, &told); err != nil {
		klog.ErrorS(err, "read the KwatchConfig notice",
			"component", "crd-watcher")
		return
	}
	if !o.invalid {
		if told != "" {
			_ = disk.put(stateOverlayRejected, "")
		}
		return
	}
	if told == o.rejected {
		return
	}
	if err := disk.put(stateOverlayRejected, o.rejected); err != nil {
		klog.ErrorS(err, "record the KwatchConfig notice",
			"component", "crd-watcher")
		return
	}
	if send != nil {
		send(overlayNoticeText(o))
	}
}
