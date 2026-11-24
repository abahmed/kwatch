package dashboard

import "net/url"

// stateAsStatus lets "state" name the list filter too: when there is no
// "status" and "state" is the single value active, resolved or all, it
// is the status ("status" stays the alias). Any other "state" is still
// the per-state filter (open, flapping ...). It returns q unchanged
// unless it moves a value.
func stateAsStatus(q url.Values) url.Values {
	v := q["state"]
	if q.Has("status") || len(v) != 1 {
		return q
	}
	switch v[0] {
	case stateActive, stateResolved, "all":
		out := url.Values{}
		for k, vs := range q {
			out[k] = vs
		}
		out.Del("state")
		out.Set("status", v[0])
		return out
	}
	return q
}
