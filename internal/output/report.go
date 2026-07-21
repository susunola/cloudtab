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

func (r Report) Total() float64 {
	var t float64
	for _, res := range r.Resources {
		for _, c := range res.Components {
			t += c.MonthlyCost
		}
	}
	return t
}

func Render(w io.Writer, r Report, format string) error {
	switch format {
	case "json":
		return json.NewEncoder(w).Encode(r)
	case "table", "":
		return renderTable(w, r)
	}
	return fmt.Errorf("unknown format %q", format)
}

func renderTable(w io.Writer, r Report) error {
	t := tablewriter.NewWriter(w)
	t.SetHeader([]string{"Resource", "Component", "Monthly (CNY)"})
	for _, res := range r.Resources {
		for i, c := range res.Components {
			addr := ""
			if i == 0 {
				addr = res.Address
			}
			t.Append([]string{addr, c.Name, fmt.Sprintf("%.2f", c.MonthlyCost)})
		}
	}
	t.SetFooter([]string{"", "TOTAL", fmt.Sprintf("%.2f", r.Total())})
	t.Render()
	if len(r.Skipped) > 0 {
		fmt.Fprintln(w, "\nSkipped resources:")
		for _, s := range r.Skipped {
			fmt.Fprintf(w, "  - %s (%s): %s\n", s.Address, s.Type, s.Reason)
		}
	}
	return nil
}
