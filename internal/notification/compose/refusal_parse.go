package compose

import (
	"regexp"
	"strconv"
	"strings"
)

// The API server says why it refused a create in its own words, and a
// FailedCreate finding quotes it. These read the numbers out of that
// text and write the refusing object's line: the quota's resource,
// used, hard and requested; the limit range's bound and what was asked.

// maxRefusalResources bounds how many resources one quota line lists.
const maxRefusalResources = 3

// quotaExceeded matches "exceeded quota: NAME, requested: a=1Gi, used:
// a=31Gi, limited: a=32Gi". Resources inside one part are joined with
// a comma and no space.
var quotaExceeded = regexp.MustCompile(
	`exceeded quota: (\S+?), requested: (\S+?), used: (\S+?), ` +
		`limited: ([^\s"]+)`)

// limitBound matches "maximum memory usage per Container is 1Gi, but
// limit is 2Gi" and its "minimum ... but request is ..." twin.
var limitBound = regexp.MustCompile(
	`(maximum|minimum) (\S+) usage per (Container|Pod|` +
		`PersistentVolumeClaim) is ([^\s,"]+), but (limit|request) ` +
		`is ([^\s,"]+)`)

// limitRatio matches "memory max limit to request ratio per Container
// is 2, but provided ratio is 4.000000".
var limitRatio = regexp.MustCompile(
	`(\S+) max limit to request ratio per (Container|Pod) is ` +
		`([^\s,"]+), but provided ratio is ([^\s,"]+)`)

// quotaLine writes the quota the message names, or "" when it names
// none. A non-empty only keeps a quota of that name.
func quotaLine(message, only string) string {
	m := quotaExceeded.FindStringSubmatch(message)
	if m == nil || (only != "" && m[1] != only) {
		return ""
	}
	asked := quotaItems(m[2])
	used, hard := quotaItems(m[3]).index(), quotaItems(m[4]).index()
	var parts []string
	for _, r := range asked {
		if len(parts) == maxRefusalResources {
			break
		}
		parts = append(parts, r.name+" "+used[r.name]+" of "+
			hard[r.name]+" used; this pod asks "+r.value)
	}
	if len(parts) == 0 {
		return ""
	}
	return "ResourceQuota " + m[1] + ": " + strings.Join(parts, "; ")
}

// quotaItem is one "resource=value" of a quota list.
type quotaItem struct{ name, value string }

// quotaList is the resources of one quota list, in the order written.
type quotaList []quotaItem

// quotaItems reads "a=1,b=2".
func quotaItems(list string) quotaList {
	var out quotaList
	for _, item := range strings.Split(list, ",") {
		if name, value, ok := strings.Cut(item, "="); ok {
			out = append(out, quotaItem{name, value})
		}
	}
	return out
}

// index lets a quotaList be read by resource name.
func (l quotaList) index() map[string]string {
	out := map[string]string{}
	for _, p := range l {
		out[p.name] = p.value
	}
	return out
}

// limitLine writes the limit range bound the message names, or "".
func limitLine(message, rangeName string) string {
	subject := "A LimitRange of the namespace"
	if rangeName != "" {
		subject = "LimitRange " + rangeName
	}
	if m := limitBound.FindStringSubmatch(message); m != nil {
		bound := map[string]string{"maximum": "max", "minimum": "min"}
		return subject + ": " + bound[m[1]] + " " + m[2] + " " + m[4] +
			" per " + strings.ToLower(m[3]) + "; " + asker(m[3]) +
			" asks " + m[6] + " (" + m[5] + ")"
	}
	if m := limitRatio.FindStringSubmatch(message); m != nil {
		return subject + ": " + m[1] + " limit may be at most " + m[3] +
			"x its request per " + strings.ToLower(m[2]) + "; " + asker(m[2]) +
			" has " + ratio(m[4]) + "x"
	}
	return ""
}

// asker says who asked for the refused amount.
func asker(scope string) string {
	if scope == "Container" {
		return "a container"
	}
	return "the pod"
}

// ratio writes the API server's "4.000000" as "4".
func ratio(text string) string {
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return text
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}
