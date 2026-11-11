package kubeclient

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Answers counts, per resource, the informer lists and watches the API
// server answered with success. A watch that failed reads it to learn that a
// later list or watch got through: the resource version alone does not
// say so for a kind whose objects rarely change, because a relist then
// returns the version it already had. It is safe for concurrent use; a
// nil Answers counts nothing.
type Answers struct {
	mu sync.Mutex
	n  map[answerKey]uint64
}

type answerKey struct{ group, resource string }

// Count is how many lists and watches of the resource were answered
// with success so far. The core group is "".
func (a *Answers) Count(group, resource string) uint64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n[answerKey{group, resource}]
}

func (a *Answers) add(group, resource string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.n == nil {
		a.n = map[answerKey]uint64{}
	}
	a.n[answerKey{group, resource}]++
}

// Wrap returns rt with every successful list or watch counted. It is
// what rest.Config.Wrap takes.
func (a *Answers) Wrap(rt http.RoundTripper) http.RoundTripper {
	return answerTripper{next: rt, answers: a}
}

type answerTripper struct {
	next    http.RoundTripper
	answers *Answers
}

// RoundTrip implements http.RoundTripper. It only reads the request
// line and the status: the response goes back untouched.
func (t answerTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK ||
		r.Method != http.MethodGet || r.URL == nil {
		return resp, err
	}
	if !informerRequest(r.URL.Query()) {
		return resp, err
	}
	if group, resource, ok := collectionOf(r.URL.Path); ok {
		t.answers.add(group, resource)
	}
	return resp, err
}

// informerRequest reports a list or watch shaped as an informer sends
// it: a watch, or a list that names the resource version it starts
// from ("0" at first, then the last one synced). Another client on the
// same configuration that lists a resource ("get the pods of one
// namespace now") names none, and its answer says nothing about
// whether the informer's watch gets through.
func informerRequest(query url.Values) bool {
	switch query.Get("watch") {
	case "true", "1":
		return true
	}
	return query.Has("resourceVersion")
}

// collectionOf names the resource a path lists or watches:
// /api/v1/pods, /apis/apps/v1/deployments, or either inside one
// namespace. A path to one object or a subresource is not a collection.
// Whatever precedes /api or /apis (a proxy prefix) is skipped.
func collectionOf(path string) (group, resource string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		switch part {
		case "api":
			// api/<version>/...
			parts = parts[min(i+2, len(parts)):]
		case "apis":
			// apis/<group>/<version>/...
			if i+1 < len(parts) {
				group = parts[i+1]
			}
			parts = parts[min(i+3, len(parts)):]
		default:
			continue
		}
		switch {
		case len(parts) == 1:
			return group, parts[0], true
		case len(parts) == 3 && parts[0] == "namespaces":
			return group, parts[2], true
		}
		return "", "", false
	}
	return "", "", false
}
