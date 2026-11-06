package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/status"
)

// kwatchDeliveryWindow is how far back /api/kwatch counts sends.
const kwatchDeliveryWindow = 24 * time.Hour

// KwatchResponse is the /api/kwatch response: kwatch's own status page
// in one read. Each part is absent while its source is: Coverage
// without the sources, StateFile, Restarts24h and OutboxPending while
// no session runs, FailedSendsLastHour without delivery.
type KwatchResponse struct {
	Coverage            *KwatchCoverage   `json:"coverage,omitempty"`
	Delivery            []ChannelDelivery `json:"delivery"`
	DeliveryWindowHours int               `json:"deliveryWindowHours"`
	StateFile           *StateFileStatus  `json:"stateFile,omitempty"`
	Resources           *ResourcesStatus  `json:"resources,omitempty"`
	// LikelyMisses are the incidents of the last seven days whose
	// recovery followed a change to something other than the cause they
	// named (the cause self-check), newest first; Pro only.
	LikelyMisses        []status.LikelyMiss `json:"likelyMisses,omitempty"`
	Restarts24h         *int                `json:"restarts24h,omitempty"`
	OutboxPending       *int                `json:"outboxPending,omitempty"`
	FailedSendsLastHour *int                `json:"failedSendsLastHour,omitempty"`
}

// KwatchCoverage is what kwatch reads, in totals (see /api/coverage
// for the lists): Watched counts running watches and Synced those
// whose cache holds the whole list; Unsynced counts kinds whose first
// list has not finished, Unreadable those the API refuses and
// NotWatched those turned off or over a budget. Total is every kind
// kwatch knows of: Watched + Unreadable + NotWatched.
type KwatchCoverage struct {
	Watched            int       `json:"watched"`
	Synced             int       `json:"synced"`
	Total              int       `json:"total"`
	Unreadable         int       `json:"unreadable"`
	Unsynced           int       `json:"unsynced"`
	NotWatched         int       `json:"notWatched"`
	AllNamespaces      bool      `json:"allNamespaces"`
	Namespaces         int       `json:"namespaces"`
	ExcludedNamespaces int       `json:"excludedNamespaces"`
	SyncedAt           time.Time `json:"syncedAt,omitzero"`
}

// ChannelDelivery is one alert channel's incident messages of the last
// day, from the send trail kwatch keeps per incident: Sent were
// accepted, Failed given up or handed to the fallback, Retrying still
// being sent. LastSuccess and LastError are the channel's own health,
// summaries and digests included.
type ChannelDelivery struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Kind        string    `json:"kind"`
	Sent        int       `json:"sent"`
	Failed      int       `json:"failed"`
	Retrying    int       `json:"retrying"`
	LastSuccess time.Time `json:"lastSuccess,omitzero"`
	LastError   time.Time `json:"lastError,omitzero"`
}

// StateFileStatus is the state file's size. CapBytes is the size cap
// the compactor keeps the stored data under, 0 when it is unknown
// (CapKnown false).
type StateFileStatus struct {
	Bytes    int64 `json:"bytes"`
	CapBytes int64 `json:"capBytes,omitempty"`
	CapKnown bool  `json:"capKnown"`
}

// kwatchHandler serves GET /api/kwatch.
func (s *Server) kwatchReport() *KwatchResponse {
	out := KwatchResponse{
		DeliveryWindowHours: int(kwatchDeliveryWindow / time.Hour),
		FailedSendsLastHour: s.failedLastHour(),
	}
	if src := s.sources.coverage.Load(); src != nil {
		out.Coverage = coverageTotals(withTotals((*src)()))
	}
	if self, ok := s.selfReport(); ok {
		restarts, pending := len(self.Restarts), self.OutboxPending
		out.Restarts24h, out.OutboxPending = &restarts, &pending
		out.StateFile = &StateFileStatus{Bytes: self.StateFileBytes,
			CapBytes: s.opts.StateCapBytes,
			CapKnown: s.opts.StateCapBytes > 0}
	}
	var health []ChannelHealth
	if h := s.sources.health.Load(); h != nil {
		health = (*h)()
	}
	var items []incident.Incident
	if reader := s.sources.incidents.Load(); reader != nil {
		items = (*reader).Incidents()
	}
	var trail deliveryTrailSource
	if t := s.sources.trail.Load(); t != nil {
		trail = *t
	}
	out.Delivery = channelDeliveries(health, items, trail, s.now.Now())
	out.Resources = s.resources(out.StateFile)
	if src := s.sources.status.Load(); src != nil {
		out.LikelyMisses = (*src)().LikelyMisses
	}
	return &out
}

// coverageTotals is the totals of c.
func coverageTotals(c CoverageResponse) *KwatchCoverage {
	t := c.KindTotals
	return &KwatchCoverage{Watched: c.Kinds.Watched, Synced: c.Kinds.Synced,
		Total:      c.Kinds.Watched + t.Unreadable + t.NotWatched,
		Unreadable: t.Unreadable, Unsynced: t.Unsynced,
		NotWatched: t.NotWatched, AllNamespaces: c.Namespaces.All,
		Namespaces:         c.Namespaces.Watched,
		ExcludedNamespaces: c.Namespaces.Excluded, SyncedAt: c.SyncedAt}
}

// channelDeliveries counts the last day's sends per channel from the
// trail of every incident held (at most maxInsightScan), with every
// channel the delivery health knows listed even without sends. Sorted
// by name.
func channelDeliveries(
	health []ChannelHealth, items []incident.Incident,
	trail deliveryTrailSource, now time.Time,
) []ChannelDelivery {
	// One entry per provider whatever its case: the delivery health
	// names it "slack" where the send trail says "Slack".
	byProvider := map[string]*ChannelDelivery{}
	entry := func(provider string) *ChannelDelivery {
		key := strings.ToLower(provider)
		c := byProvider[key]
		if c == nil {
			c = &ChannelDelivery{Provider: provider,
				Name: channelName(provider),
				Kind: channelKind(provider, false)}
			byProvider[key] = c
		}
		return c
	}
	for _, h := range health {
		c := entry(cmp.Or(h.Type, h.Channel))
		c.LastSuccess, c.LastError = h.LastSuccess, h.LastError
	}
	if trail != nil {
		countSends(entry, items, trail, now.Add(-kwatchDeliveryWindow))
	}
	out := make([]ChannelDelivery, 0, len(byProvider))
	for _, c := range byProvider {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b ChannelDelivery) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.Provider, b.Provider))
	})
	return out
}

// countSends adds each provider's sends since from to its entry.
func countSends(
	entry func(string) *ChannelDelivery, items []incident.Incident,
	trail deliveryTrailSource, from time.Time,
) {
	for i, p := range items {
		if i >= maxInsightScan {
			return
		}
		for _, a := range trail(p.ID) {
			if a.Channel == "" || a.At.Before(from) {
				continue
			}
			c := entry(a.Channel)
			if a.Pager {
				c.Kind = channelPager
			}
			switch a.Result {
			case deliveredOK:
				c.Sent++
			case "failed":
				c.Failed++
			case "retrying":
				c.Retrying++
			}
		}
	}
}
