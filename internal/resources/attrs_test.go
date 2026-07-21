package resources

import "testing"

func TestGetStr(t *testing.T) {
	tests := []struct {
		m    map[string]interface{}
		k    string
		want string
	}{
		{map[string]interface{}{"name": "foo"}, "name", "foo"},
		{map[string]interface{}{"name": 42}, "name", ""},
		{map[string]interface{}{}, "missing", ""},
		{nil, "any", ""},
	}
	for _, tt := range tests {
		if got := getStr(tt.m, tt.k); got != tt.want {
			t.Errorf("getStr(%q) = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestGetInt(t *testing.T) {
	tests := []struct {
		m    map[string]interface{}
		k    string
		want int64
	}{
		{map[string]interface{}{"n": float64(42)}, "n", 42},
		{map[string]interface{}{"n": int(42)}, "n", 42},
		{map[string]interface{}{"n": int64(99)}, "n", 99},
		{map[string]interface{}{"n": "123"}, "n", 123},
		{map[string]interface{}{"n": "45.6"}, "n", 45},
		{map[string]interface{}{"n": "hello"}, "n", 0},
		{map[string]interface{}{}, "missing", 0},
		{nil, "any", 0},
	}
	for _, tt := range tests {
		if got := getInt(tt.m, tt.k); got != tt.want {
			t.Errorf("getInt(%q) = %d, want %d", tt.k, got, tt.want)
		}
	}
}

func TestGetBool(t *testing.T) {
	tests := []struct {
		m    map[string]interface{}
		k    string
		want bool
	}{
		{map[string]interface{}{"flag": true}, "flag", true},
		{map[string]interface{}{"flag": false}, "flag", false},
		{map[string]interface{}{"flag": "true"}, "flag", true},
		{map[string]interface{}{"flag": "false"}, "flag", false},
		{map[string]interface{}{"flag": "1"}, "flag", true},
		{map[string]interface{}{"flag": "0"}, "flag", false},
		{map[string]interface{}{}, "missing", false},
		{nil, "any", false},
	}
	for _, tt := range tests {
		if got := getBool(tt.m, tt.k); got != tt.want {
			t.Errorf("getBool(%q) = %v, want %v", tt.k, got, tt.want)
		}
	}
}

func TestGetStrMissingKey(t *testing.T) {
	// getStr on a map with a key of wrong type returns empty string.
	m := map[string]interface{}{"val": 123}
	if got := getStr(m, "val"); got != "" {
		t.Errorf("getStr(non-string) = %q, want empty", got)
	}
	if got := getStr(m, "nope"); got != "" {
		t.Errorf("getStr(missing) = %q, want empty", got)
	}
}
