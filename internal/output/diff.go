package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

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
	Address       string   `json:"address"`
	Type          string   `json:"type"`
	Kind          DiffKind `json:"kind"`
	BeforeMonthly float64  `json:"before_monthly"`
	AfterMonthly  float64  `json:"after_monthly"`
	DeltaMonthly  float64  `json:"delta_monthly"`
}

type DiffReport struct {
	Resources   []ResourceDiff    `json:"resources"`
	Skipped     []SkippedResource `json:"skipped"`
	BeforeTotal float64           `json:"before_total"`
	AfterTotal  float64           `json:"after_total"`
	DeltaTotal  float64           `json:"delta_total"`
	Currency    string            `json:"currency"`
}

// ComputeDiff pairs resources by address and computes monthly delta.
func ComputeDiff(before, after Report) DiffReport {
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

	// Infer currency from both reports. When the before and after agree on a
	// single non-empty currency use it; otherwise leave empty (mixed / unknown).
	// uniformCurrency on the combined set gives the right answer even when one
	// side is empty (e.g. a pure-add diff).
	out.Currency, _ = uniformCurrency(Report{Resources: append(
		append([]ResourceCost(nil), before.Resources...),
		after.Resources...,
	)})

	for addr, br := range bm {
		seen[addr] = true
		bTotal := ResourceCostTotal(br)
		if ar, ok := am[addr]; ok {
			aTotal := ResourceCostTotal(ar)
			kind := DiffSame
			if math.Abs(aTotal-bTotal) > 1e-6 {
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
		aTotal := ResourceCostTotal(ar)
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

	// Merge skipped resources from both sides, de-duplicating by address.
	// A resource that was skipped in before and added in after (or vice versa)
	// appears once with the reason from whichever side has it.
	skipped := map[string]SkippedResource{}
	for _, s := range before.Skipped {
		skipped[s.Address] = s
	}
	for _, s := range after.Skipped {
		if _, exists := skipped[s.Address]; !exists {
			skipped[s.Address] = s
		}
	}
	out.Skipped = make([]SkippedResource, 0, len(skipped))
	for _, s := range skipped {
		out.Skipped = append(out.Skipped, s)
	}
	sort.Slice(out.Skipped, func(i, j int) bool {
		return out.Skipped[i].Address < out.Skipped[j].Address
	})

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
	// Only sum into a single TOTAL when every priced component shares one
	// currency; mixing e.g. CNY + USD would produce a meaningless figure, so we
	// show a dash instead (matching the non-diff renderTable behavior).
	if d.Currency == "" {
		t.SetFooter([]string{"", "TOTAL (mixed currencies)", "", "-", "-", "-"})
	} else {
		t.SetFooter([]string{"", "TOTAL", "",
			fmt.Sprintf("%.2f", d.BeforeTotal),
			fmt.Sprintf("%.2f", d.AfterTotal),
			fmt.Sprintf("%+.2f", d.DeltaTotal),
		})
	}
	t.Render()
	if len(d.Skipped) > 0 {
		fmt.Fprintln(w, "\nSkipped resources:")
		for _, s := range d.Skipped {
			fmt.Fprintf(w, "  - %s (%s): %s\n", s.Address, s.Type, s.Reason)
		}
	}
	return nil
}

func renderDiffMarkdown(w io.Writer, d DiffReport) error {
	fmt.Fprintln(w, "## 💰 cloudtab — Cloud cost estimate")
	fmt.Fprintln(w)
	curStr := d.Currency
	if curStr == "" {
		curStr = "(mixed currencies)"
	}
	fmt.Fprintf(w, "**Monthly change:** `%+.2f %s` (before `%.2f` → after `%.2f`)\n\n",
		d.DeltaTotal, curStr, d.BeforeTotal, d.AfterTotal)
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
	fmt.Fprintln(w, "> Priced via provider-specific pricing APIs (Tencent InquiryPrice / AWS Price List / Alibaba BSS / Huawei BSS).")
	if len(d.Skipped) > 0 {
		addrs := make([]string, len(d.Skipped))
		for i, s := range d.Skipped {
			addrs[i] = s.Address
		}
		fmt.Fprintf(w, "\n> ⚠️ %d resource(s) skipped (unsupported type): `%s`\n",
			len(d.Skipped), strings.Join(addrs, "`, `"))
	}
	return nil
}
