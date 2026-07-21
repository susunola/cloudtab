// Package parser loads Terraform plan JSON into a normalized resource list.
package parser

import (
	"encoding/json"
	"fmt"
	"os"
)

// PlannedResource is one row we hand to the pricing engine.
// It intentionally keeps the raw attribute map so per-type mappers can pull
// what they need (instance_type, data_disks, charge_type, etc.).
type PlannedResource struct {
	Address string                 `json:"address"`
	Type    string                 `json:"type"` // e.g. "tencentcloud_instance"
	Name    string                 `json:"name"`
	After   map[string]interface{} `json:"after"` // resource_changes[].change.after
	Region  string                 `json:"region,omitempty"`
}

type Plan struct {
	FormatVersion string            `json:"format_version"`
	Resources     []PlannedResource `json:"-"`
}

// LoadPlanJSON parses `terraform show -json <plan>` output.
// Only "create" and "update" actions contribute to cost;
// "delete" resources are handled in diff mode.
func LoadPlanJSON(path string) (*Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		FormatVersion   string `json:"format_version"`
		ResourceChanges []struct {
			Address string `json:"address"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Change  struct {
				Actions []string               `json:"actions"`
				After   map[string]interface{} `json:"after"`
			} `json:"change"`
		} `json:"resource_changes"`
		Configuration struct {
			ProviderConfig map[string]struct {
				Expressions map[string]struct {
					ConstantValue string `json:"constant_value"`
				} `json:"expressions"`
			} `json:"provider_config"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid plan json: %w", err)
	}

	defaultRegion := ""
	if pc, ok := doc.Configuration.ProviderConfig["tencentcloud"]; ok {
		if r, ok := pc.Expressions["region"]; ok {
			defaultRegion = r.ConstantValue
		}
	}

	p := &Plan{FormatVersion: doc.FormatVersion}
	for _, rc := range doc.ResourceChanges {
		if !contributesToCost(rc.Change.Actions) {
			continue
		}
		region := defaultRegion
		if v, ok := rc.Change.After["region"].(string); ok && v != "" {
			region = v
		}
		p.Resources = append(p.Resources, PlannedResource{
			Address: rc.Address,
			Type:    rc.Type,
			Name:    rc.Name,
			After:   rc.Change.After,
			Region:  region,
		})
	}
	return p, nil
}

func contributesToCost(actions []string) bool {
	for _, a := range actions {
		if a == "create" || a == "update" {
			return true
		}
	}
	return false
}
