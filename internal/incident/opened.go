package incident

// Fact kinds, one per kind of investigated fact.
const (
	// FactError is the first meaningful error line a crashed container
	// wrote.
	FactError = "error"
	// FactTermination is the termination message a container left.
	FactTermination = "termination"
	// FactNode lists a node's failing conditions.
	FactNode = "node"
	// FactTopMemory and FactTopCPU list a node's heaviest pods.
	FactTopMemory = "top.memory"
	FactTopCPU    = "top.cpu"
	// FactScheduler counts the nodes each scheduling blocker rejects.
	FactScheduler = "scheduler"
	// FactKeys names the config keys a change touched, never values.
	FactKeys = "keys"
	// FactEndpoints is the ready endpoint count of a webhook's Service.
	FactEndpoints = "endpoints"
	// FactWebhook is the admission failure the API server returned.
	FactWebhook = "webhook"
	// FactPull is the error class of a failed image pull.
	FactPull = "pull"
	// FactDependency is the Service a stuck init container's output
	// names, with what is wrong with it, if anything.
	FactDependency = "dependency"
)

// Fact is one fact an investigation found about an incident, such
// as the first error line of a crashed container or the config keys a
// change touched. Text is short and already redacted.
type Fact struct {
	// Kind names the kind of fact, one of the Fact* constants.
	Kind string
	// Subject names what the fact is about when that is not the
	// incident's root, such as the Service behind a webhook. Optional.
	Subject string
	Text    string
}

// maxOpened bounds the opened incidents waiting for TakeOpened, so a
// manager nobody asks never grows. The oldest are dropped first.
const maxOpened = 256

func (m *Manager) noteOpened(id string) {
	m.opened = append(m.opened, id)
	if over := len(m.opened) - maxOpened; over > 0 {
		m.opened = append(m.opened[:0:0], m.opened[over:]...)
	}
}

// TakeOpened returns detached copies of the incidents opened since the
// previous call and still settling, oldest first, and forgets them. The
// pipeline starts their investigation while they settle, so evidence is
// ready when they are announced.
func (m *Manager) TakeOpened() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Incident
	for _, id := range m.opened {
		// An incident merged into another one before it was announced is
		// gone; its members are investigated with the incident they
		// joined.
		if p := m.incidents[id]; p != nil && p.State == Settling {
			out = append(out, p.Snapshot())
		}
	}
	m.opened = nil
	return out
}
