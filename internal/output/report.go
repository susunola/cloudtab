// Package output renders reports as table / JSON.
package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

type CostComponent struct {
	Name        string  `json:"name"`
	Unit        string  `json:"unit"`
	HourlyCost  float64 `json:"hourly_cost"`
	MonthlyCost float64 `json:"monthly_cost"`
	Currency    string  `json:"currency"`
}

type ResourceCost struct {
	Address    string          `json:"address"`
	Type       string          `json:"type"`
	Components []CostComponent `json:"components"`
}

type SkippedResource struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Reason  string `json:"reason"`
}

type Report struct {
	Resources []ResourceCost    `json:"resources"`
	Skipped   []SkippedResource `json:"skipped"`
}

// Total returns the sum of MonthlyCost over all cost components.
func (r Report) Total() float64 {
	var t float64
	for _, res := range r.Resources {
		t += ResourceCostTotal(res)
	}
	return t
}

// ResourceCostTotal sums the MonthlyCost of all components in a single resource.
func ResourceCostTotal(rc ResourceCost) float64 {
	var t float64
	for _, c := range rc.Components {
		t += c.MonthlyCost
	}
	return t
}

func Render(w io.Writer, r Report, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(r)
	case "table", "":
		return renderTable(w, r)
	}
	return fmt.Errorf("unknown format %q", format)
}

func renderTable(w io.Writer, r Report) error {
	// Currency is per-component now that a single report may mix providers
	// (Tencent prices in CNY, AWS in USD). We surface it as its own column and
	// keep the monthly header generic. The TOTAL footer is only meaningful when
	// every component shares one currency; when currencies are mixed we show a
	// dash instead of summing incomparable amounts.
	t := tablewriter.NewWriter(w)
	t.SetHeader([]string{"Resource", "Component", "Monthly", "Currency"})
	for _, res := range r.Resources {
		for i, c := range res.Components {
			addr := ""
			if i == 0 {
				addr = res.Address
			}
			t.Append([]string{addr, c.Name, fmt.Sprintf("%.2f", c.MonthlyCost), c.Currency})
		}
	}
	if len(r.Resources) == 0 {
		// No priced resources: there is no currency to total, so show a flat
		// zero rather than a misleading "mixed currencies" label.
		t.SetFooter([]string{"", "TOTAL", "0.00", ""})
	} else if cur, uniform := uniformCurrency(r); uniform {
		t.SetFooter([]string{"", "TOTAL", fmt.Sprintf("%.2f", r.Total()), cur})
	} else {
		t.SetFooter([]string{"", "TOTAL (mixed currencies)", "-", ""})
	}
	t.Render()
	if len(r.Skipped) > 0 {
		fmt.Fprintln(w, "\nSkipped resources:")
		for _, s := range r.Skipped {
			fmt.Fprintf(w, "  - %s (%s): %s\n", s.Address, s.Type, s.Reason)
		}
	}
	return nil
}

// uniformCurrency reports whether every priced component in the report shares a
// single non-empty currency, returning that currency. When components disagree
// (a mixed Tencent+AWS report) or there are none, it returns ("", false) so the
// caller can avoid summing incomparable amounts. Empty currencies are ignored
// (zero-cost placeholder lines shouldn't force a "mixed" verdict).
func uniformCurrency(r Report) (string, bool) {
	cur := ""
	for _, res := range r.Resources {
		for _, c := range res.Components {
			if c.Currency == "" {
				continue
			}
			if cur == "" {
				cur = c.Currency
				continue
			}
			if c.Currency != cur {
				return "", false
			}
		}
	}
	if cur == "" {
		return "", false
	}
	return cur, true
}
