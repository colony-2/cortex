package gitstate

import "fmt"

func mapFromAny(input map[string]interface{}, key string) (map[string]interface{}, error) {
	val, ok := input[key]
	if !ok {
		return nil, fmt.Errorf("missing %s in input", key)
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be a map", key)
	}
	return m, nil
}

func mapFromAnyOptional(input map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, ok := input[key]
	if !ok {
		return nil, false
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return m, true
}

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
