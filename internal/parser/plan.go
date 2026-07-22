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

	// Each Terraform provider declares its own default region in its provider
	// block. We read them per provider so the correct default is applied by
	// resource type: tencentcloud_* resources default to the tencentcloud
	// provider's region, aws_* resources to the aws provider's region.
	providerRegion := func(name string) string {
		if pc, ok := doc.Configuration.ProviderConfig[name]; ok {
			if r, ok := pc.Expressions["region"]; ok {
				return r.ConstantValue
			}
		}
		return ""
	}
	tencentRegion := providerRegion("tencentcloud")
	awsRegion := providerRegion("aws")

	p := &Plan{FormatVersion: doc.FormatVersion}
	for _, rc := range doc.ResourceChanges {
		if !contributesToCost(rc.Change.Actions) {
			continue
		}
		// Pick the provider-block default by resource type, then let an explicit
		// per-resource "region" attribute (Tencent resources carry one; AWS
		// resources generally do not) override it.
		region := defaultRegionForType(rc.Type, tencentRegion, awsRegion)
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

// defaultRegionForType returns the provider-block default region appropriate
// for a resource type. AWS resource types are prefixed "aws_"; everything else
// is treated as Tencent Cloud (the historical default), preserving prior
// behaviour for every tencentcloud_* type.
func defaultRegionForType(tfType, tencentRegion, awsRegion string) string {
	if ProviderForType(tfType) == "aws" {
		return awsRegion
	}
	return tencentRegion
}

// ProviderForType maps a Terraform resource type to the pricing provider that
// serves it, based on the type's provider prefix. It returns "aws" for aws_*
// types and "tencentcloud" for everything else (the historical default).
func ProviderForType(tfType string) string {
	if len(tfType) >= 4 && tfType[:4] == "aws_" {
		return "aws"
	}
	return "tencentcloud"
}

func contributesToCost(actions []string) bool {
	for _, a := range actions {
		if a == "create" || a == "update" {
			return true
		}
	}
	return false
}
