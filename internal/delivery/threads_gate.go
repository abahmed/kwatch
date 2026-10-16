package delivery

import deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"

// ThreadSwitch is implemented by providers that thread an incident's
// messages and can send them unthreaded instead.
type ThreadSwitch = deliveryapi.ThreadSwitch

// applyThreadGate turns native threads off on the new providers when the
// edition does not entitle them. A provider that does not thread, such as
// a webhook or a pager, already sends every message on its own.
func (m *Manager) applyThreadGate(entries []providerEntry) {
	if m.nativeThreads {
		return
	}
	for i := range entries {
		if switcher, ok := entries[i].provider.(ThreadSwitch); ok {
			switcher.DisableThreads()
		}
	}
}
