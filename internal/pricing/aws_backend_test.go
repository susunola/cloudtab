package pricing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// fakePricing is an in-memory awsGetProductsAPI used to exercise the backend
// without touching the network. It records the last input and returns a
// scripted sequence of pages.
type fakePricing struct {
	pages    []*pricing.GetProductsOutput
	calls    int
	lastIn   *pricing.GetProductsInput
	allInput []*pricing.GetProductsInput
}

func (f *fakePricing) GetProducts(_ context.Context, in *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	f.lastIn = in
	f.allInput = append(f.allInput, in)
	out := f.pages[f.calls]
	f.calls++
	return out, nil
}

func strp(s string) *string { return &s }

// TestAWSBackendQueryBuildsServiceCodeAndFilters verifies the backend always
// prepends a ServiceCode filter and translates neutral Params filters into
// TERM_MATCH SDK filters.
func TestAWSBackendQueryBuildsServiceCodeAndFilters(t *testing.T) {
	fake := &fakePricing{pages: []*pricing.GetProductsOutput{
		{PriceList: []string{`{"sku":"A"}`, `{"sku":"B"}`}},
	}}
	b := &awsBackend{client: fake}

	req := PriceRequest{
		Provider: "aws",
		Product:  "AmazonEC2",
		Region:   "us-east-1",
		Params: map[string]interface{}{
			"Filters": []interface{}{
				map[string]interface{}{"Field": "instanceType", "Value": "m5.large"},
				map[string]interface{}{"Field": "location", "Value": "US East (N. Virginia)"},
			},
		},
	}
	raw, err := b.query(req)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}

	// ServiceCode must be set on the input.
	if fake.lastIn.ServiceCode == nil || *fake.lastIn.ServiceCode != "AmazonEC2" {
		t.Fatalf("ServiceCode = %v, want AmazonEC2", fake.lastIn.ServiceCode)
	}
	// First filter must be ServiceCode, then the two provided filters.
	if len(fake.lastIn.Filters) != 3 {
		t.Fatalf("filters len = %d, want 3", len(fake.lastIn.Filters))
	}
	assertFilter(t, fake.lastIn.Filters[0], "ServiceCode", "AmazonEC2")
	assertFilter(t, fake.lastIn.Filters[1], "instanceType", "m5.large")
	assertFilter(t, fake.lastIn.Filters[2], "location", "US East (N. Virginia)")

	// The returned bytes must be a JSON array of the raw product documents.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		t.Fatalf("result not a JSON array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("result len = %d, want 2", len(arr))
	}
}

func assertFilter(t *testing.T, f pricingtypes.Filter, field, value string) {
	t.Helper()
	if f.Type != pricingtypes.FilterTypeTermMatch {
		t.Errorf("filter %s: type = %v, want TERM_MATCH", field, f.Type)
	}
	if f.Field == nil || *f.Field != field {
		t.Errorf("filter field = %v, want %q", f.Field, field)
	}
	if f.Value == nil || *f.Value != value {
		t.Errorf("filter %s value = %v, want %q", field, f.Value, value)
	}
}

// TestAWSBackendPaginationRespectsMaxResults verifies pagination stops once the
// MaxResults cap is reached without following further NextTokens.
func TestAWSBackendPaginationRespectsMaxResults(t *testing.T) {
	fake := &fakePricing{pages: []*pricing.GetProductsOutput{
		{PriceList: []string{`{"sku":"A"}`, `{"sku":"B"}`}, NextToken: strp("more")},
		{PriceList: []string{`{"sku":"C"}`}},
	}}
	b := &awsBackend{client: fake}

	req := PriceRequest{
		Provider: "aws",
		Product:  "AmazonEC2",
		Params:   map[string]interface{}{"MaxResults": 2},
	}
	raw, err := b.query(req)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	var arr []json.RawMessage
	_ = json.Unmarshal(raw, &arr)
	if len(arr) != 2 {
		t.Fatalf("collected %d, want 2 (capped)", len(arr))
	}
	if fake.calls != 1 {
		t.Fatalf("GetProducts called %d times, want 1 (cap reached on first page)", fake.calls)
	}
}

// TestAWSBackendFollowsPaginationToken verifies multiple pages are collected
// when under the cap and a NextToken is present.
func TestAWSBackendFollowsPaginationToken(t *testing.T) {
	fake := &fakePricing{pages: []*pricing.GetProductsOutput{
		{PriceList: []string{`{"sku":"A"}`}, NextToken: strp("p2")},
		{PriceList: []string{`{"sku":"B"}`}},
	}}
	b := &awsBackend{client: fake}

	raw, err := b.query(PriceRequest{Provider: "aws", Product: "AmazonS3", Params: map[string]interface{}{"MaxResults": 100}})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	var arr []json.RawMessage
	_ = json.Unmarshal(raw, &arr)
	if len(arr) != 2 {
		t.Fatalf("collected %d, want 2 across pages", len(arr))
	}
	if fake.calls != 2 {
		t.Fatalf("GetProducts called %d times, want 2", fake.calls)
	}
	// Second call must carry the NextToken from the first page.
	if fake.allInput[1].NextToken == nil || *fake.allInput[1].NextToken != "p2" {
		t.Errorf("second call NextToken = %v, want p2", fake.allInput[1].NextToken)
	}
}

// TestAWSBackendMissingServiceCode rejects a request without a Product.
func TestAWSBackendMissingServiceCode(t *testing.T) {
	b := &awsBackend{client: &fakePricing{pages: []*pricing.GetProductsOutput{{}}}}
	if _, err := b.query(PriceRequest{Provider: "aws"}); err == nil {
		t.Fatal("expected error for missing ServiceCode, got nil")
	}
}

// TestBuildAWSFiltersValidation covers the neutral->SDK filter conversion edge
// cases: nil params, wrong list type, wrong item type, and missing Field.
func TestBuildAWSFiltersValidation(t *testing.T) {
	// nil params -> just the ServiceCode filter.
	f, err := buildAWSFilters("AmazonEC2", nil)
	if err != nil || len(f) != 1 {
		t.Fatalf("nil params: got %d filters, err %v", len(f), err)
	}
	// Filters not a list.
	if _, err := buildAWSFilters("AmazonEC2", map[string]interface{}{"Filters": "nope"}); err == nil {
		t.Error("expected error when Filters is not a list")
	}
	// Item not a map.
	if _, err := buildAWSFilters("AmazonEC2", map[string]interface{}{"Filters": []interface{}{"nope"}}); err == nil {
		t.Error("expected error when filter item is not a map")
	}
	// Missing Field.
	bad := map[string]interface{}{"Filters": []interface{}{map[string]interface{}{"Value": "x"}}}
	if _, err := buildAWSFilters("AmazonEC2", bad); err == nil {
		t.Error("expected error when filter Field is missing")
	}
}

// TestAWSMaxResultsClamp checks the MaxResults parsing and clamping to [1,100].
func TestAWSMaxResultsClamp(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{nil, 100},
		{5, 5},
		{float64(3), 3},
		{"7", 7},
		{0, 1},
		{-4, 1},
		{500, 100},
	}
	for _, c := range cases {
		var params map[string]interface{}
		if c.in != nil {
			params = map[string]interface{}{"MaxResults": c.in}
		}
		if got := awsMaxResults(params); got != c.want {
			t.Errorf("awsMaxResults(%v) = %d, want %d", c.in, got, c.want)
		}
	}
	_ = aws.String // keep import used across builds
}
