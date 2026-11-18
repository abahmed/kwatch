package servicenow

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Defaults follow the out-of-box incident table: state 6 is Resolved,
// 2 is In Progress, and "Solution provided" is a stock resolution code.
const (
	defaultTable        = "incident"
	defaultNoteField    = "work_notes"
	defaultResolveState = "6"
	defaultReopenState  = "2"
	defaultCloseCode    = "Solution provided"
	defaultCloseNotes   = "Resolved by kwatch: the failure recovered."
)

// tiers are the keys of the urgency and impact maps.
var tiers = []string{"page", "notify", "low"}

// defaultLevels maps each tier to a ServiceNow urgency or impact value
// (1 high, 2 medium, 3 low).
var defaultLevels = map[string]string{"page": "1", "notify": "2", "low": "3"}

// tableName is a ServiceNow table or column name.
var tableName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

// stateValue is a choice value of the state column.
var stateValue = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)

// settings is the validated configuration of one provider.
type settings struct {
	base            string
	table           string
	user            string
	password        string
	clientID        string
	clientSecret    string
	assignmentGroup string
	caller          string
	category        string
	urgency         map[string]string
	impact          map[string]string
	fields          map[string]string
	noteField       string
	resolveState    string
	reopenState     string
	closeCode       string
	closeNotes      string
}

// parseSettings reads and checks the provider configuration. Errors name
// the setting, never its value, which may be a credential.
func parseSettings(config map[string]interface{}) (settings, error) {
	s := settings{
		table:        text(config, "table", defaultTable),
		user:         text(config, "user", ""),
		password:     text(config, "password", ""),
		clientID:     text(config, "clientID", ""),
		clientSecret: text(config, "clientSecret", ""),
		noteField:    text(config, "noteField", defaultNoteField),
		resolveState: text(config, "resolveState", defaultResolveState),
		closeCode:    text(config, "closeCode", defaultCloseCode),
		closeNotes:   text(config, "closeNotes", defaultCloseNotes),
		reopenState:  reopenState(config),
	}
	s.assignmentGroup = text(config, "assignmentGroup", "")
	s.caller = text(config, "caller", "")
	s.category = text(config, "category", "")
	base, err := InstanceBase(text(config, "instanceURL", ""))
	if err != nil {
		return settings{}, err
	}
	s.base = base
	if err := s.checkNames(); err != nil {
		return settings{}, err
	}
	if err := s.checkAuth(); err != nil {
		return settings{}, err
	}
	return s.withMaps(config)
}

// withMaps reads the urgency, impact and fields mappings.
func (s settings) withMaps(config map[string]interface{}) (settings, error) {
	var err error
	if s.urgency, err = levels(config["urgency"], "urgency"); err != nil {
		return settings{}, err
	}
	if s.impact, err = levels(config["impact"], "impact"); err != nil {
		return settings{}, err
	}
	if s.fields, err = customFields(config["fields"]); err != nil {
		return settings{}, err
	}
	return s, nil
}

// reopenState is the state a recurrence sets; an explicit empty value
// turns reopening off, so a recurrence files a new incident.
func reopenState(config map[string]interface{}) string {
	raw, set := config["reopenState"]
	if !set || raw == nil {
		return defaultReopenState
	}
	return strings.TrimSpace(scalar(raw))
}

// InstanceBase checks the instance address and returns it without a
// trailing slash. It must be https, name a host and carry no path,
// query, fragment or user information: credentials go in their own
// settings.
func InstanceBase(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("instanceURL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", errors.New("instanceURL must be an https address " +
			"such as https://example.service-now.com")
	}
	if parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" {
		return "", errors.New("instanceURL must be the instance address " +
			"only, without a path, query or user information")
	}
	return "https://" + parsed.Host, nil
}

func (s settings) checkNames() error {
	if !tableName.MatchString(s.table) {
		return errors.New("table must be a table name such as incident")
	}
	if !tableName.MatchString(s.noteField) {
		return errors.New("noteField must be a column name such as " +
			"work_notes or comments")
	}
	if !stateValue.MatchString(s.resolveState) {
		return errors.New("resolveState must be a state value such as 6")
	}
	if s.reopenState != "" && !stateValue.MatchString(s.reopenState) {
		return errors.New("reopenState must be a state value such as 2, " +
			"or empty to file a new incident on a recurrence")
	}
	return nil
}

// checkAuth accepts exactly one complete credential pair: basic (user and
// password) or OAuth client credentials (clientID and clientSecret).
func (s settings) checkAuth() error {
	basic := s.user != "" || s.password != ""
	oauth := s.clientID != "" || s.clientSecret != ""
	switch {
	case basic && oauth:
		return errors.New("set either user and password, or clientID " +
			"and clientSecret, not both")
	case basic && (s.user == "" || s.password == ""):
		return errors.New("basic authentication needs both user and " +
			"password")
	case oauth && (s.clientID == "" || s.clientSecret == ""):
		return errors.New("OAuth needs both clientID and clientSecret")
	case !basic && !oauth:
		return errors.New("set user and password, or clientID and " +
			"clientSecret")
	}
	return nil
}

func (s settings) oauth() bool { return s.clientID != "" }

// levels reads a tier map (page, notify, low) over the defaults. Values
// are ServiceNow's 1 (high) to 3 (low).
func levels(raw interface{}, name string) (map[string]string, error) {
	out := make(map[string]string, len(defaultLevels))
	for tier, level := range defaultLevels {
		out[tier] = level
	}
	given, err := stringMap(raw, name)
	if err != nil {
		return nil, err
	}
	for tier, level := range given {
		if !isTier(tier) {
			return nil, fmt.Errorf("%s.%s is not a tier; use page, "+
				"notify or low", name, tier)
		}
		if n, err := strconv.Atoi(level); err != nil || n < 1 || n > 3 {
			return nil, fmt.Errorf("%s.%s must be 1, 2 or 3", name, tier)
		}
		out[tier] = level
	}
	return out, nil
}

func isTier(name string) bool {
	for _, tier := range tiers {
		if tier == name {
			return true
		}
	}
	return false
}

// customFields reads the extra columns set on a new incident.
func customFields(raw interface{}) (map[string]string, error) {
	fields, err := stringMap(raw, "fields")
	if err != nil {
		return nil, err
	}
	for name := range fields {
		if !tableName.MatchString(name) {
			return nil, fmt.Errorf("fields.%s is not a column name", name)
		}
	}
	return fields, nil
}
