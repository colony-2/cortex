package gitstate

func stringFromMap(m map[string]interface{}, key string) (string, bool) {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok && str != "" {
			return str, true
		}
	}
	return "", false
}

func boolFromMap(m map[string]interface{}, key string) (bool, bool) {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b, true
		}
	}
	return false, false
}

func intFromMap(m map[string]interface{}, key string) (int, bool) {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v, true
		case int32:
			return int(v), true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		}
	}
	return 0, false
}
