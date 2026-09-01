package statuswatch

// defaultConditionRules contains only condition states that represent a
// durable failure. Informational conditions are intentionally ignored.
func defaultConditionRules() map[string]map[string]bool {
	return map[string]map[string]bool{
		"Ready": {
			"False":   true,
			"Unknown": true,
		},
		"Available": {
			"False":   true,
			"Unknown": true,
		},
		"Degraded":    {"True": true},
		"Progressing": {"False": true},
	}
}

func defaultConditionRulesWithAdmission() map[string]map[string]bool {
	rules := defaultConditionRules()
	rules["Valid"] = map[string]bool{"False": true, "Unknown": true}
	return rules
}
