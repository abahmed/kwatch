package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/health"
)

// selfSource is kwatch's view of itself; ok is false while no session
// runs a pipeline.
type selfSource func() (health.SelfReport, bool)

// failedSource counts the message sends that failed since a time.
type failedSource func(since time.Time) int

// deliveryDetailSource is what the pagers and channels hold of the
// incident with an ID.
type deliveryDetailSource func(id string) DeliveryDetail

// SelfStatus is kwatch's view of itself on /api/status (see
// health.SelfReport): when it started, how its last session ended, its
// restarts of the last day, its last contact with the API server, its
// state file and outbox, whether messages link to the dashboard, pod
// log access and the first sync's progress.
type SelfStatus struct {
	health.SelfReport
	// DashboardLinks adds a sentence saying why links are off.
	DashboardLinks SelfLinks `json:"dashboardLinks"`
}

// SelfLinks is whether messages link to the dashboard; Text says in a
// plain sentence why they do not, absent while they do.
type SelfLinks struct {
	health.LinkStatus
	Text string `json:"text,omitempty"`
}

// linksOffText is the sentence for links that are off for reason,
// in the words /status uses (health.LinkReasonText).
func linksOffText(links health.LinkStatus) string {
	if links.On {
		return ""
	}
	if text := health.LinkReasonText(links.Reason); text != "" {
		return "Alerts carry no dashboard link: " + text + "."
	}
	return "Alerts carry no dashboard link."
}

// SetSelf sets kwatch's view of itself; nil removes it, and /api/status
// leaves self out.
func (s *Server) SetSelf(src func() (health.SelfReport, bool)) {
	if src == nil {
		s.sources.self.Store(nil)
		return
	}
	v := selfSource(src)
	s.sources.self.Store(&v)
}

// SetFailedSince sets the count of failed sends since a time; nil
// leaves failedLastHour out of /api/status.
func (s *Server) SetFailedSince(src func(since time.Time) int) {
	if src == nil {
		s.sources.failed.Store(nil)
		return
	}
	v := failedSource(src)
	s.sources.failed.Store(&v)
}

// SetDeliveryDetail sets what the pagers and channels hold of an
// incident; nil leaves pager and sent out of the incident page.
func (s *Server) SetDeliveryDetail(src func(id string) DeliveryDetail) {
	if src == nil {
		s.sources.delivery.Store(nil)
		return
	}
	v := deliveryDetailSource(src)
	s.sources.delivery.Store(&v)
}

// selfReport is kwatch's view of itself; ok is false without one.
func (s *Server) selfReport() (health.SelfReport, bool) {
	src := s.sources.self.Load()
	if src == nil {
		return health.SelfReport{}, false
	}
	return (*src)()
}

// selfStatus is the self part of /api/status; nil without a report.
func (s *Server) selfStatus() *SelfStatus {
	report, ok := s.selfReport()
	if !ok {
		return nil
	}
	return &SelfStatus{SelfReport: report, DashboardLinks: SelfLinks{
		LinkStatus: report.DashboardLinks,
		Text:       linksOffText(report.DashboardLinks)}}
}

// failedLastHour counts the sends that failed in the hour before now;
// nil without delivery.
func (s *Server) failedLastHour() *int {
	src := s.sources.failed.Load()
	if src == nil {
		return nil
	}
	n := (*src)(s.now.Now().Add(-time.Hour))
	return &n
}
