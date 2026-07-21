package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/olekukonko/tablewriter"
)

// DiffKind is + (added), - (removed), ~ (changed), = (unchanged).
type DiffKind string

const (
	DiffAdd    DiffKind = "+"
	DiffRemove DiffKind = "-"
	DiffChange DiffKind = "~"
	DiffSame   DiffKind = "="
)

type ResourceDiff struct {
	Address        string   `json:"address"`
	Type           string   `json:"type"`
	Kind           DiffKind `json:"kind"`
	BeforeMonthly  float64  `json:"before_monthly"`
	AfterMonthly   float64  `json:"after_monthly"`
	DeltaMonthly   float64  `json:"delta_monthly"`
}

type DiffReport struct {
	Resources     []ResourceDiff `json:"resources"`
	BeforeTotal   float64        `json:"before_total"`
	AfterTotal    float64        `json:"after_total"`
	DeltaTotal    float64        `json:"delta_total"`
	Currency      string         `json:"currency"`
}

// ComputeDiff pairs resources by address and computes monthly delta.
func ComputeDiff(before, after Report) DiffReport {
	sumComps := func(rc ResourceCost) float64 {
		var t float64
		for _, c := range rc.Components {
			t += c.MonthlyCost
		}
		return t
	}
	idx := func(rep Report) map[string]ResourceCost {
		m := make(map[string]ResourceCost, len(rep.Resources))
		for _, r := range rep.Resources {
			m[r.Address] = r
		}
		return m
	}
	bm, am := idx(before), idx(after)
	seen := map[string]bool{}
	var out DiffReport
	out.Currency = "CNY"

	for addr, br := range bm {
		seen[addr] = true
		bTotal := sumComps(br)
		if ar, ok := am[addr]; ok {
			aTotal := sumComps(ar)
			kind := DiffSame
			if aTotal != bTotal {
				kind = DiffChange
			}
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: br.Type, Kind: kind,
				BeforeMonthly: bTotal, AfterMonthly: aTotal, DeltaMonthly: aTotal - bTotal,
			})
		} else {
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: br.Type, Kind: DiffRemove,
				BeforeMonthly: bTotal, AfterMonthly: 0, DeltaMonthly: -bTotal,
			})
		}
	}
	for addr, ar := range am {
		if seen[addr] {
			continue
		}
		aTotal := sumComps(ar)
		out.Resources = append(out.Resources, ResourceDiff{
			Address: addr, Type: ar.Type, Kind: DiffAdd,
			BeforeMonthly: 0, AfterMonthly: aTotal, DeltaMonthly: aTotal,
		})
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		return out.Resources[i].Address < out.Resources[j].Address
	})
	out.BeforeTotal = before.Total()
	out.AfterTotal = after.Total()
	out.DeltaTotal = out.AfterTotal - out.BeforeTotal
	return out
}

// RenderDiff writes the diff report as table / json / markdown (PR-comment friendly).
func RenderDiff(w io.Writer, d DiffReport, format string) error {
	switch format {
	case "json":
		return json.NewEncoder(w).Encode(d)
	case "markdown":
		return renderDiffMarkdown(w, d)
	case "table", "":
		return renderDiffTable(w, d)
	}
	return fmt.Errorf("unknown format %q", format)
}

func renderDiffTable(w io.Writer, d DiffReport) error {
	t := tablewriter.NewWriter(w)
	t.SetHeader([]string{"", "Address", "Type", "Before", "After", "Δ Monthly"})
	for _, r := range d.Resources {
		t.Append([]string{
			string(r.Kind),
			r.Address,
			r.Type,
			fmt.Sprintf("%.2f", r.BeforeMonthly),
			fmt.Sprintf("%.2f", r.AfterMonthly),
			fmt.Sprintf("%+.2f", r.DeltaMonthly),
		})
	}
	t.SetFooter([]string{"", "TOTAL", "",
		fmt.Sprintf("%.2f", d.BeforeTotal),
		fmt.Sprintf("%.2f", d.AfterTotal),
		fmt.Sprintf("%+.2f", d.DeltaTotal),
	})
	t.Render()
	return nil
}

func renderDiffMarkdown(w io.Writer, d DiffReport) error {
	fmt.Fprintln(w, "## 💰 cloudtab — Tencent Cloud cost estimate")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**Monthly change:** `%+.2f %s` (before `%.2f` → after `%.2f`)\n\n",
		d.DeltaTotal, d.Currency, d.BeforeTotal, d.AfterTotal)
	fmt.Fprintln(w, "|  | Resource | Before | After | Δ Monthly |")
	fmt.Fprintln(w, "|---|---|---:|---:|---:|")
	for _, r := range d.Resources {
		if r.Kind == DiffSame {
			continue
		}
		fmt.Fprintf(w, "| %s | `%s` | %.2f | %.2f | **%+.2f** |\n",
			r.Kind, r.Address, r.BeforeMonthly, r.AfterMonthly, r.DeltaMonthly)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> Priced via Tencent Cloud `InquiryPrice*` APIs. All amounts in "+d.Currency+".")
	return nil
}
