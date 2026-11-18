package servicenow

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// correlationDisplay names kwatch as the source of the correlation id.
const correlationDisplay = "kwatch"

// errNoRecord is an answer without the record it should hold: a
// hibernating instance or a proxy page answers 200 with HTML. Nothing
// was filed, and a retry first looks the correlation id up, so retrying
// cannot duplicate the incident.
var errNoRecord = errors.New("servicenow answered without a record; " +
	"is the instance awake and the address right?")

// CreateIncident implements issues.IncidentCreator. The marker is the
// correlation id that Find looks up, so a create retried after a lost
// answer or a restart finds the incident instead of filing another.
func (p *ServiceNow) CreateIncident(
	ctx context.Context, msg notification.Message,
	title, body, marker string,
) (string, error) {
	record := map[string]string{}
	for name, value := range p.settings.fields {
		record[name] = value
	}
	tier := tierOf(msg)
	record["short_description"] = title
	record["description"] = body
	record["correlation_id"] = marker
	record["correlation_display"] = correlationDisplay
	record["urgency"] = p.settings.urgency[tier]
	record["impact"] = p.settings.impact[tier]
	setIf(record, "assignment_group", p.settings.assignmentGroup)
	setIf(record, "caller_id", p.settings.caller)
	setIf(record, "category", p.settings.category)
	answer, err := p.call(ctx, "POST",
		p.tableURL("")+"?sysparm_fields=sys_id", record)
	if err != nil {
		return "", err
	}
	var created struct {
		Result struct {
			SysID string `json:"sys_id"`
		} `json:"result"`
	}
	if json.Unmarshal(answer, &created) != nil ||
		created.Result.SysID == "" {
		return "", errNoRecord
	}
	return created.Result.SysID, nil
}

// Create implements issues.Tracker for callers without a message; the
// issue map always uses CreateIncident.
func (p *ServiceNow) Create(
	ctx context.Context, title, body string,
) (string, error) {
	return p.CreateIncident(ctx, notification.Message{}, title, body, "")
}

// Find implements issues.Finder: the open incident whose correlation id
// is marker. A resolved incident can stay active until it is closed, so
// the resolve state is excluded too: a recurrence after the reopen
// window files a new incident instead of noting on a resolved one.
func (p *ServiceNow) Find(ctx context.Context, marker string) (string, error) {
	query := url.Values{
		"sysparm_query": {"correlation_id=" + marker + "^active=true" +
			"^state!=" + p.settings.resolveState},
		"sysparm_fields": {"sys_id"},
		"sysparm_limit":  {"1"},
	}
	answer, err := p.call(ctx, "GET",
		p.tableURL("")+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	var found struct {
		Result []struct {
			SysID string `json:"sys_id"`
		} `json:"result"`
	}
	if json.Unmarshal(answer, &found) != nil {
		return "", errNoRecord
	}
	if len(found.Result) == 0 {
		return "", nil
	}
	return found.Result[0].SysID, nil
}

// Comment implements issues.Tracker with a journal entry (work notes by
// default).
func (p *ServiceNow) Comment(ctx context.Context, id, body string) error {
	return p.patch(ctx, id, map[string]string{p.settings.noteField: body})
}

// Close implements issues.Tracker: the note, then the resolve.
func (p *ServiceNow) Close(ctx context.Context, id, body string) error {
	if err := p.Comment(ctx, id, body); err != nil {
		return err
	}
	return p.CloseState(ctx, id)
}

// CloseState implements issues.StateCloser: it resolves the incident
// with the configured state, resolution code and notes.
func (p *ServiceNow) CloseState(ctx context.Context, id string) error {
	return p.patch(ctx, id, map[string]string{
		"state":       p.settings.resolveState,
		"close_code":  p.settings.closeCode,
		"close_notes": p.settings.closeNotes,
	})
}

// CanReopen is checked by the issue map: with reopenState set to ""
// a recurrence files a new incident.
func (p *ServiceNow) CanReopen() bool { return p.settings.reopenState != "" }

// Reopen implements issues.Reopener.
func (p *ServiceNow) Reopen(ctx context.Context, id string) error {
	if p.settings.reopenState == "" {
		return nil
	}
	return p.patch(ctx, id,
		map[string]string{"state": p.settings.reopenState})
}

func (p *ServiceNow) patch(
	ctx context.Context, id string, fields map[string]string,
) error {
	_, err := p.call(ctx, "PATCH",
		p.tableURL(id)+"?sysparm_fields=sys_id", fields)
	return err
}

// tableURL is the Table API address of the table or of one record.
func (p *ServiceNow) tableURL(id string) string {
	address := p.settings.base + "/api/now/table/" + p.settings.table
	if id != "" {
		address += "/" + url.PathEscape(id)
	}
	return address
}

func setIf(record map[string]string, name, value string) {
	if value != "" {
		record[name] = value
	}
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// call sends one Table API request. With OAuth, a 401 means the cached
// token was revoked or expired early: it is fetched again once.
func (p *ServiceNow) call(
	ctx context.Context, method, address string, payload interface{},
) ([]byte, error) {
	answer, err := p.send(ctx, method, address, payload)
	if p.settings.oauth() && unauthorized(err) {
		p.dropToken()
		answer, err = p.send(ctx, method, address, payload)
	}
	return answer, err
}

func (p *ServiceNow) send(
	ctx context.Context, method, address string, payload interface{},
) ([]byte, error) {
	var body []byte
	if payload != nil {
		var err error
		if body, err = json.Marshal(payload); err != nil {
			return nil, transport.Permanent(err)
		}
	}
	r := transport.Request{
		Provider: p.Name(), Method: method, URL: address, Body: body,
		Headers: map[string]string{"Accept": "application/json"},
	}
	if body != nil {
		r.ContentType = "application/json"
	}
	if err := p.authorize(ctx, &r); err != nil {
		return nil, err
	}
	return p.sender.Send(ctx, r)
}

// unauthorized reports a 401 from the Table API. A refused token request
// is not one: fetching again would be refused the same way.
func unauthorized(err error) bool {
	var refused *tokenError
	if errors.As(err, &refused) {
		return false
	}
	code, ok := transport.StatusOf(err)
	return ok && code == 401 // Unauthorized
}
