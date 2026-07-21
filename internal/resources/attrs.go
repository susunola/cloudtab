package resources

import (
	"strconv"
)

// attrHelpers provide safe, consistent reads from Terraform attribute maps.
// All mappers receive attributes as map[string]interface{} from the JSON plan,
// where numeric values are decoded as float64 by encoding/json.

func getStr(m map[string]interface{}, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, k string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
		if parsedFloat, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(parsedFloat)
		}
	}
	return 0
}

func getBool(m map[string]interface{}, k string) bool {
	if m == nil {
		return false
	}
	switch v := m[k].(type) {
	case bool:
		return v
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return false
}

// firstZone returns the first availability zone from a "zones" list attribute
// (e.g. tencentcloud_mariadb_instance takes a []string). It tolerates both a
// []interface{} (typical JSON plan decoding) and a plain string, returning ""
// when no zone can be found.
func firstZone(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	switch v := m["zones"].(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				return s
			}
		}
	case []string:
		for _, s := range v {
			if s != "" {
				return s
			}
		}
	case string:
		return v
	}
	return ""
}
