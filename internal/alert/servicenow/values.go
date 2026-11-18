package servicenow

import (
	"fmt"
	"strconv"
	"strings"
)

// text reads a trimmed string setting, or fallback when it is unset or
// blank.
func text(config map[string]interface{}, key, fallback string) string {
	value := strings.TrimSpace(scalar(config[key]))
	if value == "" {
		return fallback
	}
	return value
}

// scalar renders a YAML scalar (string, number, boolean) as text; any
// other value reads as empty.
func scalar(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// stringMap reads a mapping of scalar values. Unset is an empty map.
func stringMap(raw interface{}, name string) (map[string]string, error) {
	out := map[string]string{}
	switch m := raw.(type) {
	case nil:
		return out, nil
	case map[string]string:
		for k, v := range m {
			out[k] = strings.TrimSpace(v)
		}
		return out, nil
	case map[string]interface{}:
		for k, v := range m {
			value := scalar(v)
			if value == "" && v != "" {
				return nil, fmt.Errorf("%s.%s must be text or a number",
					name, k)
			}
			out[k] = strings.TrimSpace(value)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s must be a mapping of names to values", name)
}
