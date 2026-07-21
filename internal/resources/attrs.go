package resources

// attrHelpers provide safe, consistent reads from Terraform attribute maps.
// All mappers receive attributes as map[string]interface{} from the JSON plan,
// where numeric values are decoded as float64 by encoding/json.

func getStr(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func getBool(m map[string]interface{}, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}
