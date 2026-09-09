package model

import "time"

// PersistedIncident is a lightweight serializable subset of Incident,
// stored in the kwatch-incidents ConfigMap to survive restarts.
type PersistedIncident struct {
	Key            IncidentKey     `json:"key"`
	Fingerprint    string          `json:"fingerprint,omitempty"`
	Reason         string          `json:"reason"`
	Namespace      string          `json:"namespace"`
	Name           string          `json:"name"`
	Resource       string          `json:"resource"`
	Count          int             `json:"count"`
	FirstSeen      time.Time       `json:"firstSeen"`
	LastSeen       time.Time       `json:"lastSeen"`
	Resources      map[string]bool `json:"resources"`
	PeakResources  int             `json:"peakResources"`
	OwnerKind      string          `json:"ownerKind"`
	RestartCount   int             `json:"restartCount"`
	Hint           string          `json:"hint"`
	Facts          Facts           `json:"facts,omitempty"`
	Severity       Severity        `json:"severity"`
	State          IncidentState   `json:"state"`
	ResolveAt      time.Time       `json:"resolveAt,omitempty"`
	NotifiedSig    string          `json:"notifiedSig"`
	LastNotifiedAt time.Time       `json:"lastNotifiedAt"`
	RenotifyCount  int             `json:"renotifyCount"`
	SuppressedBy   IncidentKey     `json:"suppressedBy,omitempty"`
	Transient      bool            `json:"transient,omitempty"`
}

// ToPersisted converts an Incident into its serializable subset.
func (inc *Incident) ToPersisted() PersistedIncident {
	resources := make(map[string]bool, len(inc.Resources))
	for k, v := range inc.Resources {
		resources[k] = v
	}
	return PersistedIncident{
		Key:            inc.Key,
		Fingerprint:    inc.Fingerprint,
		Reason:         inc.Reason,
		Namespace:      inc.Namespace,
		Name:           inc.Name,
		Resource:       inc.Resource,
		Count:          inc.Count,
		FirstSeen:      inc.FirstSeen,
		LastSeen:       inc.LastSeen,
		Resources:      resources,
		PeakResources:  inc.PeakResources,
		OwnerKind:      inc.OwnerKind,
		RestartCount:   inc.RestartCount,
		Hint:           inc.Hint,
		Facts:          inc.Facts.clone(),
		Severity:       inc.Severity,
		State:          inc.State,
		ResolveAt:      inc.ResolveAt,
		NotifiedSig:    inc.NotifiedSig,
		LastNotifiedAt: inc.LastNotifiedAt,
		RenotifyCount:  inc.RenotifyCount,
		SuppressedBy:   inc.SuppressedBy,
		Transient:      inc.Transient,
	}
}

// ToIncident converts a PersistedIncident back to a full Incident.
func (pi *PersistedIncident) ToIncident() *Incident {
	// Every refresh path writes into Resources; a nil map from an older or
	// hand-edited ConfigMap would panic the engine on the first event.
	resources := pi.Resources
	if resources == nil {
		resources = make(map[string]bool)
	}
	inc := &Incident{
		Subject: Subject{
			Key:         pi.Key,
			Fingerprint: pi.Fingerprint,
			Reason:      pi.Reason,
			Namespace:   pi.Namespace,
			Name:        pi.Name,
			Resource:    pi.Resource,
			OwnerKind:   pi.OwnerKind,
			Transient:   pi.Transient,
		},
		// Object and Owner are references, not stored text: they are
		// recovered from the persisted name below so an incident written by
		// an older kwatch restores with the same identity as one this
		// process created.
		Status: Status{
			Count:         pi.Count,
			FirstSeen:     pi.FirstSeen,
			LastSeen:      pi.LastSeen,
			Resources:     resources,
			PeakResources: pi.PeakResources,
			RestartCount:  pi.RestartCount,
			Severity:      pi.Severity,
			State:         pi.State,
			ResolveAt:     pi.ResolveAt,
			Containers:    make(map[string]bool),
			LastUpdate:    pi.LastSeen,
		},
		Evidence: Evidence{
			Hint:  pi.Hint,
			Facts: pi.Facts.clone(),
		},
		Attribution: Attribution{
			SuppressedBy: pi.SuppressedBy,
		},
		Delivery: Delivery{
			NotifiedSig:    pi.NotifiedSig,
			LastNotifiedAt: pi.LastNotifiedAt,
			RenotifyCount:  pi.RenotifyCount,
		},
	}
	inc.SetObject()
	if pi.OwnerKind != "" {
		inc.Owner = ObjectRef{
			Kind:      pi.OwnerKind,
			Namespace: pi.Namespace,
			Name:      inc.Object.Name,
		}
	}
	return inc
}

// PersistedGroup is the serializable state of one smart group.
//
// Incidents were persisted but the groups that speak for them were not, so a
// restart forgot which incidents belonged to a group: each member then
// resolved on its own -- forty green ticks for one recovery -- and a group
// still failing re-announced itself as new.
type PersistedGroup struct {
	// GroupKey is the engine's internal grouping key ("reason|ns|owner").
	GroupKey string `json:"groupKey"`
	// IncidentKey is the synthetic incident the group notifies under.
	IncidentKey IncidentKey `json:"incidentKey"`
	Reason      string      `json:"reason"`
	Summary     string      `json:"summary"`
	// Members are the incident keys the group is waiting on, and Resolved
	// those that have already recovered.
	Members    []IncidentKey `json:"members,omitempty"`
	Resolved   []IncidentKey `json:"resolved,omitempty"`
	TotalCount int           `json:"totalCount"`
	FirstSeen  time.Time     `json:"firstSeen"`
	LastSeen   time.Time     `json:"lastSeen"`
	Severity   Severity      `json:"severity"`
	// Notified and LastNotifiedAt carry the flush state, so a restored group
	// updates its existing notification instead of creating a second one.
	Notified       bool      `json:"notified"`
	LastNotifiedAt time.Time `json:"lastNotifiedAt"`
}

// PersistedEngineState is the correlation engine's remaining working memory:
// the bookkeeping that is neither an incident nor a group, but whose loss
// after a restart is still visible to whoever reads the channel.
//
// Each field costs something specific when it is forgotten:
//   - Cooldowns: an incident that resolved just before the restart and
//     recurs just after is announced loudly instead of being revived
//     silently, which is the "resolved → crash → resolved" ping-pong the
//     cooldown exists to prevent.
//   - PodUIDs: the guard that stops a replacement Pod reusing a name from
//     being mistaken for the old one.
//   - Containers: "what happened the previous time this container died",
//     so the first alert after a restart loses its termination context.
//   - FanOut: which owner already announced itself in a grouping scope, so
//     the first owner of every scope announces individually again.
type PersistedEngineState struct {
	Cooldowns  []PersistedCooldown       `json:"cooldowns,omitempty"`
	PodUIDs    []PersistedPodUIDs        `json:"podUIDs,omitempty"`
	Containers []PersistedContainerState `json:"containers,omitempty"`
	FanOut     []PersistedFanOutWindow   `json:"fanOut,omitempty"`
}

// PersistedCooldown is one post-resolve quiet period.
type PersistedCooldown struct {
	Key     IncidentKey `json:"key"`
	Expires time.Time   `json:"expires"`
}

// PersistedPodUIDs maps an incident's Pod names to the UIDs they had, so a
// name reused by a replacement Pod is not taken for the original.
type PersistedPodUIDs struct {
	Key  IncidentKey       `json:"key"`
	UIDs map[string]string `json:"uids,omitempty"`
}

// PersistedContainerState is one container's last known termination state.
type PersistedContainerState struct {
	// Key is "namespace/pod/container" as built by lastContainerKey.
	Key       string          `json:"key"`
	IndexedAt time.Time       `json:"indexedAt"`
	State     *ContainerState `json:"state,omitempty"`
}

// PersistedFanOutWindow records which owners have already failed the same way
// in one namespace during an open grouping window.
type PersistedFanOutWindow struct {
	// Scope is "reason|namespace".
	Scope     string    `json:"scope"`
	FirstSeen time.Time `json:"firstSeen"`
	// Owners are those seen failing this way; Announced maps the ones that
	// were alerted individually to the incident they were announced under.
	Owners    []string               `json:"owners,omitempty"`
	Announced map[string]IncidentKey `json:"announced,omitempty"`
}
