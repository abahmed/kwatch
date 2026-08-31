package correlation

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) rememberPodResource(key model.IncidentKey, ev event.Event) {
	if ev.Resource != "pod" || ev.PodName == "" || ev.PodUID == "" {
		return
	}
	refs := e.podResourceUIDs[key]
	if refs == nil {
		refs = make(map[string]string)
		e.podResourceUIDs[key] = refs
	}
	refs[ev.PodName] = ev.PodUID
}
