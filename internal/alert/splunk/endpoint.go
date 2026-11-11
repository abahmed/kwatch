package splunk

import "net/url"

// eventPath is the HEC event endpoint. Operators often paste only the
// host and port, which Splunk answers with 404 for every event.
const eventPath = "/services/collector/event"

// withEventPath appends eventPath when the URL has no path of its own;
// a URL that already names a path is used as given.
func withEventPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Path != "" && u.Path != "/") {
		return raw
	}
	u.Path = eventPath
	return u.String()
}
