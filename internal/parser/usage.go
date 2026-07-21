package parser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UsageOverrides maps resource address -> arbitrary usage key/value pairs.
// Example:
//
//	tencentcloud_cos_bucket.static_site:
//	  monthly_storage_gb: 200
//	  monthly_get_requests: 1000000
//
// Unknown fields are preserved and consumed by mapper-specific Extract logic.
type UsageOverrides map[string]map[string]interface{}

// LoadUsageYAML reads usage assumptions from YAML.
// Empty path means "no overrides" and returns an empty map.
func LoadUsageYAML(path string) (UsageOverrides, error) {
	out := UsageOverrides{}
	if path == "" {
		return out, nil
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		return out, nil
	}
	if err := yaml.Unmarshal(blob, &out); err != nil {
		return nil, fmt.Errorf("invalid usage yaml: %w", err)
	}
	if out == nil {
		out = UsageOverrides{}
	}
	return out, nil
}
