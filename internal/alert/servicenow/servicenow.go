package servicenow

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// Field limits of the stock incident table: short_description holds 160
// characters, description and journal entries 4000. bodyRoom leaves space
// for the reference line the issue map adds to a new incident.
const (
	titleLimit = 160
	bodyLimit  = 4000
	bodyRoom   = 64
)

// ServiceNow files one incident per kwatch incident.
type ServiceNow struct {
	issues      *issues.Map
	sender      transport.Sender
	settings    settings
	tokens      tokens
	now         func() time.Time
	clusterName string
}

// NewServiceNow returns the provider, or nil (with the reason logged)
// when the settings are unusable.
func NewServiceNow(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *ServiceNow {
	s, err := parseSettings(config)
	if err != nil {
		klog.InfoS("initializing servicenow with invalid settings",
			"reason", err.Error())
		return nil
	}
	klog.InfoS("initializing servicenow",
		"instance", transport.LogURL(s.base), "table", s.table,
		"oauth", s.oauth(), "noteField", s.noteField,
		"reopens", s.reopenState != "")
	now := clock.Require(dependencies.Clock).Now
	return &ServiceNow{
		issues:      issues.NewMap(now).ForCluster(clusterName),
		sender:      transport.NewSender(dependencies),
		settings:    s,
		now:         now,
		clusterName: clusterName,
	}
}

// Name returns the provider name.
func (p *ServiceNow) Name() string { return "ServiceNow" }

// SendIncident opens one incident per kwatch incident, adds a note on
// updates and resolves it on recovery.
func (p *ServiceNow) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	body := issues.RichBody(msg, notification.PlainDialect(), "----")
	return p.issues.Deliver(ctx, p, msg,
		notification.Truncate(oneLine(issues.Title(msg, titleLimit)),
			titleLimit),
		notification.Truncate(body, bodyLimit-bodyRoom))
}

// SendMessage treats a plain message as a notice, which never opens an
// incident.
func (p *ServiceNow) SendMessage(ctx context.Context, msg string) error {
	return p.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper.
func (p *ServiceNow) SkipsPlainMessages() bool { return true }

// HasThread implements delivery.ThreadLookup.
func (p *ServiceNow) HasThread(key string) bool {
	return p.issues.HasThread(key)
}

// SnapshotThreads implements delivery.ThreadStateProvider: the stored
// value is the incident's sys_id.
func (p *ServiceNow) SnapshotThreads() map[string]string {
	return p.issues.SnapshotThreads()
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (p *ServiceNow) RestoreThreads(saved map[string]string) {
	p.issues.RestoreThreads(saved)
}

// tierOf names the kwatch tier of a message for the urgency and impact
// maps.
func tierOf(msg notification.Message) string {
	switch msg.Status {
	case notification.StatusCritical:
		return "page"
	case notification.StatusWarning, notification.StatusFlapping:
		return "notify"
	}
	return "low"
}
