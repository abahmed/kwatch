package app

// selfGrantedMax bounds the saved list of parts seen working; the
// parts are a fixed set of component names, so it is never reached in
// practice.
const selfGrantedMax = 64

// selfReasonOptionalDenied is the health reason of an optional
// permission the API refuses.
const selfReasonOptionalDenied = "optional_permission_denied"

// recordGrants saves each part seen working, and reports whether the
// saved set grew.
func (a *selfAlerter) recordGrants() bool {
	if a.proven == nil {
		return false
	}
	changed := false
	for _, part := range a.proven() {
		if a.state.Granted[part] || len(a.state.Granted) >= selfGrantedMax {
			continue
		}
		if a.state.Granted == nil {
			a.state.Granted = map[string]bool{}
		}
		a.state.Granted[part] = true
		changed = true
	}
	return changed
}

// dropNeverGranted removes the optional permissions that were refused
// from the first check of this installation: leaving them out was a
// choice, and the health page still lists them. A part that worked
// before, or has an open message already, stays.
func (a *selfAlerter) dropNeverGranted(current map[string]selfCondition) {
	for part, c := range current {
		if c.Reason != selfReasonOptionalDenied || a.state.Granted[part] {
			continue
		}
		if _, open := a.state.Open[part]; open {
			continue
		}
		delete(current, part)
	}
}
