package parser

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// UsageOverrides maps resource address -> arbitrary usage key/value pairs.
// Unknown fields are preserved and consumed by mapper-specific Extract logic.
type UsageOverrides map[string]map[string]interface{}

// UsageData contains either legacy overrides or typed versioned usage.
type UsageData struct {
	Version   int
	Legacy    UsageOverrides
	Resources map[string]UsageResource
}

func (u UsageData) IsVersioned() bool { return u.Version != 0 }

type UsageResource struct {
	Items map[string]UsageItem
}

type UsageItem struct {
	Quantity float64
	Unit     string
	Pricing  string
	Rate     UsageRate
}

type UsageRate struct {
	Amount   float64
	Per      float64
	Currency string
	Source   UsageSource
}

type UsageSource struct {
	Kind       string
	Reference  string
	AsOf       string
	Confidence string
}

type usageDocument struct {
	Version   int                      `yaml:"version"`
	Resources map[string]usageResource `yaml:"resources"`
}

type usageResource struct {
	Items map[string]usageItem `yaml:"items"`
}

type usageItem struct {
	Quantity *float64   `yaml:"quantity"`
	Unit     *string    `yaml:"unit"`
	Pricing  *string    `yaml:"pricing"`
	Rate     *usageRate `yaml:"rate"`
}

type usageRate struct {
	Amount   *float64     `yaml:"amount"`
	Per      *float64     `yaml:"per"`
	Currency *string      `yaml:"currency"`
	Source   *usageSource `yaml:"source"`
}

type usageSource struct {
	Kind       *string `yaml:"kind"`
	Reference  *string `yaml:"reference"`
	AsOf       *string `yaml:"as_of"`
	Confidence *string `yaml:"confidence"`
}

// LoadUsageYAML retains the legacy unversioned API.
func LoadUsageYAML(path string) (UsageOverrides, error) {
	usage, err := LoadUsageFile(path)
	if err != nil {
		return nil, err
	}
	if usage.IsVersioned() {
		return nil, fmt.Errorf("versioned usage requires typed usage data")
	}
	return usage.Legacy, nil
}

// LoadUsageFile reads either legacy unversioned overrides or strict version 1 usage.
func LoadUsageFile(path string) (UsageData, error) {
	empty := UsageData{Legacy: UsageOverrides{}, Resources: map[string]UsageResource{}}
	if path == "" {
		return empty, nil
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return UsageData{}, err
	}
	if len(bytes.TrimSpace(blob)) == 0 {
		return empty, nil
	}

	node, err := singleUsageDocument(blob)
	if err != nil {
		return UsageData{}, err
	}
	version, versioned, err := usageVersion(node)
	if err != nil {
		return UsageData{}, err
	}
	if !versioned {
		var legacy UsageOverrides
		if err := yaml.Unmarshal(blob, &legacy); err != nil {
			return UsageData{}, fmt.Errorf("invalid usage yaml: %w", err)
		}
		if legacy == nil {
			legacy = UsageOverrides{}
		}
		return UsageData{Legacy: legacy, Resources: map[string]UsageResource{}}, nil
	}
	if version != 1 {
		return UsageData{}, fmt.Errorf("unsupported usage version %d", version)
	}

	var raw usageDocument
	dec := yaml.NewDecoder(bytes.NewReader(blob))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return UsageData{}, fmt.Errorf("invalid usage yaml: %w", err)
	}
	return validateUsageDocument(raw)
}

func singleUsageDocument(blob []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(blob))
	var first *yaml.Node
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid usage yaml: %w", err)
		}
		if len(node.Content) == 0 {
			continue
		}
		if first != nil {
			return nil, fmt.Errorf("invalid usage yaml: multiple YAML documents are not supported")
		}
		copy := node
		first = &copy
	}
	if first == nil {
		return &yaml.Node{}, nil
	}
	return first, nil
}

func usageVersion(doc *yaml.Node) (int, bool, error) {
	if doc == nil || len(doc.Content) == 0 || len(doc.Content[0].Content) == 0 {
		return 0, false, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return 0, false, fmt.Errorf("invalid usage yaml: top level must be a mapping")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "version" {
			continue
		}
		var version int
		if err := root.Content[i+1].Decode(&version); err != nil {
			return 0, true, fmt.Errorf("invalid usage version: %w", err)
		}
		return version, true, nil
	}
	return 0, false, nil
}

func validateUsageDocument(raw usageDocument) (UsageData, error) {
	if raw.Resources == nil {
		return UsageData{}, fmt.Errorf("versioned usage resources are required")
	}
	out := UsageData{Version: 1, Legacy: UsageOverrides{}, Resources: make(map[string]UsageResource, len(raw.Resources))}
	for address, resource := range raw.Resources {
		if strings.TrimSpace(address) == "" {
			return UsageData{}, fmt.Errorf("usage resource address must not be empty")
		}
		if resource.Items == nil || len(resource.Items) == 0 {
			return UsageData{}, fmt.Errorf("usage resource %q items are required", address)
		}
		items := make(map[string]UsageItem, len(resource.Items))
		for id, item := range resource.Items {
			validated, err := validateUsageItem(address, id, item)
			if err != nil {
				return UsageData{}, err
			}
			items[id] = validated
		}
		out.Resources[address] = UsageResource{Items: items}
	}
	return out, nil
}

func validateUsageItem(address, id string, item usageItem) (UsageItem, error) {
	prefix := fmt.Sprintf("usage resource %q item %q", address, id)
	if strings.TrimSpace(id) == "" {
		return UsageItem{}, fmt.Errorf("%s id must not be empty", prefix)
	}
	if item.Quantity == nil || !finite(*item.Quantity) || *item.Quantity < 0 {
		return UsageItem{}, fmt.Errorf("%s quantity must be a positive-or-zero finite number", prefix)
	}
	if item.Unit == nil || strings.TrimSpace(*item.Unit) == "" {
		return UsageItem{}, fmt.Errorf("%s unit is required", prefix)
	}
	if item.Pricing == nil || strings.TrimSpace(*item.Pricing) != "supplied" {
		return UsageItem{}, fmt.Errorf("%s pricing must be supplied", prefix)
	}
	if item.Rate == nil {
		return UsageItem{}, fmt.Errorf("%s rate is required", prefix)
	}
	rate, err := validateUsageRate(prefix, *item.Rate)
	if err != nil {
		return UsageItem{}, err
	}
	return UsageItem{Quantity: *item.Quantity, Unit: strings.TrimSpace(*item.Unit), Pricing: "supplied", Rate: rate}, nil
}

func validateUsageRate(prefix string, rate usageRate) (UsageRate, error) {
	if rate.Amount == nil || !finite(*rate.Amount) || *rate.Amount < 0 {
		return UsageRate{}, fmt.Errorf("%s rate amount must be a nonnegative finite number", prefix)
	}
	if rate.Per == nil || !finite(*rate.Per) || *rate.Per <= 0 {
		return UsageRate{}, fmt.Errorf("%s rate per must be a positive finite number", prefix)
	}
	if rate.Currency == nil || strings.TrimSpace(*rate.Currency) == "" {
		return UsageRate{}, fmt.Errorf("%s rate currency is required", prefix)
	}
	if rate.Source == nil {
		return UsageRate{}, fmt.Errorf("%s rate source is required", prefix)
	}
	source, err := validateUsageSource(prefix, *rate.Source)
	if err != nil {
		return UsageRate{}, err
	}
	return UsageRate{Amount: *rate.Amount, Per: *rate.Per, Currency: strings.ToUpper(strings.TrimSpace(*rate.Currency)), Source: source}, nil
}

func validateUsageSource(prefix string, source usageSource) (UsageSource, error) {
	if source.Kind == nil || !oneOf(strings.TrimSpace(*source.Kind), "provider_documentation", "contract", "historical_bill", "other") {
		return UsageSource{}, fmt.Errorf("%s source kind is invalid", prefix)
	}
	if source.Reference == nil || strings.TrimSpace(*source.Reference) == "" {
		return UsageSource{}, fmt.Errorf("%s source reference is required", prefix)
	}
	if source.AsOf == nil {
		return UsageSource{}, fmt.Errorf("%s source as_of is required", prefix)
	}
	asOf := strings.TrimSpace(*source.AsOf)
	parsed, err := time.Parse("2006-01-02", asOf)
	if err != nil || parsed.Format("2006-01-02") != asOf {
		return UsageSource{}, fmt.Errorf("%s source as_of must be YYYY-MM-DD", prefix)
	}
	if source.Confidence == nil || !oneOf(strings.TrimSpace(*source.Confidence), "high", "medium", "low") {
		return UsageSource{}, fmt.Errorf("%s source confidence is invalid", prefix)
	}
	return UsageSource{Kind: strings.TrimSpace(*source.Kind), Reference: strings.TrimSpace(*source.Reference), AsOf: asOf, Confidence: strings.TrimSpace(*source.Confidence)}, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
