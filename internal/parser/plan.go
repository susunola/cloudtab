// Package parser loads Terraform plan JSON into a normalized resource list.
package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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

type providerConfigDefinition struct {
	Expressions map[string]struct {
		ConstantValue string `json:"constant_value"`
	} `json:"expressions"`
}

type configurationResource struct {
	Address           string `json:"address"`
	ProviderConfigKey string `json:"provider_config_key"`
}

type configurationModuleCall struct {
	Module *configurationModule `json:"module"`
}

type configurationModule struct {
	Resources   []configurationResource            `json:"resources"`
	ModuleCalls map[string]configurationModuleCall `json:"module_calls"`
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
			ProviderConfig map[string]providerConfigDefinition `json:"provider_config"`
			RootModule     configurationModule                 `json:"root_module"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid plan json: %w", err)
	}

	providerRegions := providerRegionMap(doc.Configuration.ProviderConfig)
	resourceProviders := resourceProviderConfigKeys(doc.Configuration.RootModule)

	p := &Plan{FormatVersion: doc.FormatVersion}
	for _, rc := range doc.ResourceChanges {
		if !contributesToCost(rc.Change.Actions) {
			continue
		}
		region := resolveResourceRegion(rc.Address, rc.Type, rc.Change.After, resourceProviders, providerRegions)
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

// LoadPlanInventoryJSON parses the final managed-resource inventory from
// planned_values.root_module. Older plan JSON without that root falls back to
// LoadPlanJSON's create/update resource_changes behavior.
func LoadPlanInventoryJSON(path string) (*Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		FormatVersion string `json:"format_version"`
		PlannedValues struct {
			RootModule json.RawMessage `json:"root_module"`
		} `json:"planned_values"`
		Configuration struct {
			ProviderConfig map[string]providerConfigDefinition `json:"provider_config"`
			RootModule     configurationModule                 `json:"root_module"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid plan json: %w", err)
	}
	if doc.PlannedValues.RootModule == nil {
		return LoadPlanJSON(path)
	}

	type plannedModule struct {
		Resources []struct {
			Address string                 `json:"address"`
			Mode    string                 `json:"mode"`
			Type    string                 `json:"type"`
			Name    string                 `json:"name"`
			Values  map[string]interface{} `json:"values"`
		} `json:"resources"`
		ChildModules []plannedModule `json:"child_modules"`
	}
	var root plannedModule
	if err := json.Unmarshal(doc.PlannedValues.RootModule, &root); err != nil {
		return nil, fmt.Errorf("invalid plan json: %w", err)
	}

	providerRegions := providerRegionMap(doc.Configuration.ProviderConfig)
	resourceProviders := resourceProviderConfigKeys(doc.Configuration.RootModule)

	plan := &Plan{FormatVersion: doc.FormatVersion}
	var walk func(plannedModule)
	walk = func(module plannedModule) {
		for _, resource := range module.Resources {
			if resource.Mode != "managed" {
				continue
			}
			region := resolveResourceRegion(resource.Address, resource.Type, resource.Values, resourceProviders, providerRegions)
			plan.Resources = append(plan.Resources, PlannedResource{
				Address: resource.Address,
				Type:    resource.Type,
				Name:    resource.Name,
				After:   resource.Values,
				Region:  region,
			})
		}
		for _, child := range module.ChildModules {
			walk(child)
		}
	}
	walk(root)
	return plan, nil
}

func providerRegionMap(configs map[string]providerConfigDefinition) map[string]string {
	regions := make(map[string]string, len(configs))
	for key, config := range configs {
		if expression, ok := config.Expressions["region"]; ok {
			regions[key] = strings.TrimSpace(expression.ConstantValue)
		}
	}
	return regions
}

func resourceProviderConfigKeys(root configurationModule) map[string]string {
	providers := map[string]string{}
	var walk func(configurationModule, string)
	walk = func(module configurationModule, prefix string) {
		for _, resource := range module.Resources {
			address := normalizeResourceAddress(resource.Address)
			providers[address] = resource.ProviderConfigKey
			if prefix != "" && !strings.HasPrefix(address, prefix+".") {
				providers[prefix+"."+address] = resource.ProviderConfigKey
			}
		}
		callNames := make([]string, 0, len(module.ModuleCalls))
		for name := range module.ModuleCalls {
			callNames = append(callNames, name)
		}
		sort.Strings(callNames)
		for _, name := range callNames {
			call := module.ModuleCalls[name]
			if call.Module == nil {
				continue
			}
			childPrefix := "module." + name
			if prefix != "" {
				childPrefix = prefix + "." + childPrefix
			}
			walk(*call.Module, childPrefix)
		}
	}
	walk(root, "")
	return providers
}

func normalizeResourceAddress(address string) string {
	var normalized strings.Builder
	normalized.Grow(len(address))
	inIndex := false
	inQuote := false
	escaped := false
	for i := 0; i < len(address); i++ {
		char := address[i]
		if !inIndex {
			if char == '[' {
				inIndex = true
				continue
			}
			normalized.WriteByte(char)
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if inQuote && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			inQuote = !inQuote
			continue
		}
		if char == ']' && !inQuote {
			inIndex = false
		}
	}
	return normalized.String()
}

func resolveResourceRegion(address, tfType string, values map[string]interface{}, resourceProviders, providerRegions map[string]string) string {
	region := ""
	if providerKey := resourceProviders[normalizeResourceAddress(address)]; providerKey != "" {
		region = providerRegions[providerKey]
	} else {
		region = fallbackProviderRegion(terraformProviderName(tfType), providerRegions)
	}
	if explicit, ok := values["region"].(string); ok && explicit != "" {
		region = explicit
	}
	return region
}

func fallbackProviderRegion(providerName string, providerRegions map[string]string) string {
	if region := providerRegions[providerName]; region != "" {
		return region
	}
	keys := make([]string, 0, len(providerRegions))
	for key, region := range providerRegions {
		if strings.HasPrefix(key, providerName+".") && region != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return providerRegions[keys[0]]
}

func terraformProviderName(tfType string) string {
	switch ProviderForType(tfType) {
	case "aws":
		return "aws"
	case "alibaba":
		return "alicloud"
	case "huawei":
		return "huaweicloud"
	default:
		return "tencentcloud"
	}
}

// ProviderForType maps a Terraform resource type to the pricing provider that
// serves it, based on the type's provider prefix.
func ProviderForType(tfType string) string {
	if len(tfType) >= 4 && tfType[:4] == "aws_" {
		return "aws"
	}
	if len(tfType) >= 12 && tfType[:12] == "huaweicloud_" {
		return "huawei"
	}
	if len(tfType) >= 9 && tfType[:9] == "alicloud_" {
		return "alibaba"
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
