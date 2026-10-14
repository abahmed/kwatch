package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// parseHeaders reads the headers setting: a list of name and value pairs.
// An unset setting is no headers. Any entry that is malformed, or a header
// the HTTP client would refuse, is an error, so a bad entry fails startup
// instead of leaving the provider without its credentials.
func parseHeaders(raw interface{}) ([]KeyValue, error) {
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, errors.New("headers must be a list of name/value pairs")
	}
	headers := make([]KeyValue, 0, len(list))
	for i, entry := range list {
		var pair KeyValue
		if err := roundTrip(entry, &pair); err != nil {
			return nil, fmt.Errorf("headers[%d]: %w", i, err)
		}
		if err := transport.ValidateHeader(pair.Name, pair.Value); err != nil {
			return nil, fmt.Errorf("headers[%d]: %w", i, err)
		}
		headers = append(headers, pair)
	}
	return headers, nil
}

// parseBasicAuth reads the basicAuth setting. Unset, or empty, is no auth.
// Setting only one of username and password is an error: the request would
// go out without credentials and be refused every time.
func parseBasicAuth(raw interface{}) (Authentication, error) {
	var auth Authentication
	if raw == nil {
		return auth, nil
	}
	if err := roundTrip(raw, &auth); err != nil {
		return Authentication{}, fmt.Errorf("basicAuth: %w", err)
	}
	if (auth.UserName == "") != (auth.Password == "") {
		return Authentication{}, errors.New(
			"basicAuth needs both username and password, or neither")
	}
	return auth, nil
}

// roundTrip decodes a loosely typed setting into out. The error names the
// kind of mistake, not the value, which may be a secret.
func roundTrip(raw interface{}, out interface{}) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return errors.New("the setting cannot be read")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return errors.New("must be a mapping")
	}
	if json.Unmarshal(data, out) != nil {
		return errors.New("a field has the wrong type; use text values")
	}
	return nil
}

// warnPlainHTTP warns when credentials travel over http://, where anyone on
// the path can read them. The URL may itself hold a token, so it is never
// printed; only its host is.
func warnPlainHTTP(raw string, hasCredentials bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" {
		return
	}
	if hasCredentials || parsed.User != nil {
		klog.InfoS("webhook sends credentials over plain http; use https",
			"host", parsed.Host)
	}
}
