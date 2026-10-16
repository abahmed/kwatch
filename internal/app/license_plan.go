package app

// planLabel is how a license plan is shown. Every plan the key can carry,
// the legacy "trial" value included, is a Pro license to the operator.
func planLabel(plan string) string {
	if plan == "" {
		return ""
	}
	return "Pro"
}
