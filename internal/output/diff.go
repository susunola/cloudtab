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
	Uncertain     bool     `json:"uncertain,omitempty"`
}

type CurrencyDiffTotal struct {
	Currency string  `json:"currency"`
	Before   float64 `json:"before"`
	After    float64 `json:"after"`
	Delta    float64 `json:"delta"`
}

type DiffReport struct {
	Resources        []ResourceDiff      `json:"resources"`
	Skipped          []SkippedResource   `json:"skipped"`
	BeforeTotal      float64             `json:"before_total"`
	AfterTotal       float64             `json:"after_total"`
	DeltaTotal       float64             `json:"delta_total"`
	Currency         string              `json:"currency"`
	TotalsByCurrency []CurrencyDiffTotal `json:"totals_by_currency,omitempty"`
	TotalsComplete   bool                `json:"totals_complete"`
	AggregationError string              `json:"aggregation_error,omitempty"`
	Policy           any                 `json:"policy,omitempty"`
}

func resourceMonthlyCurrency(resource ResourceCost) (float64, string, bool) {
	total := 0.0
	currency := ""
	for _, component := range resource.Components {
		total += component.MonthlyCost
		if component.Currency == "" {
			if math.Abs(component.MonthlyCost) > 1e-12 {
				return total, "", false
			}
			continue
		}
		if currency == "" {
			currency = component.Currency
		} else if component.Currency != currency {
			return total, "", false
		}
	}
	return total, currency, currency != ""
}

func monthlyTotalsByCurrency(report Report) map[string]float64 {
	totals := map[string]float64{}
	for _, resource := range report.Resources {
		for _, component := range resource.Components {
			if component.Currency != "" {
				totals[component.Currency] += component.MonthlyCost
			}
		}
	}
	return totals
}

func currencyDiffTotals(before, after Report) []CurrencyDiffTotal {
	beforeTotals, afterTotals := monthlyTotalsByCurrency(before), monthlyTotalsByCurrency(after)
	currencies := map[string]bool{}
	for currency := range beforeTotals {
		currencies[currency] = true
	}
	for currency := range afterTotals {
		currencies[currency] = true
	}
	ordered := make([]string, 0, len(currencies))
	for currency := range currencies {
		ordered = append(ordered, currency)
	}
	sort.Strings(ordered)
	out := make([]CurrencyDiffTotal, 0, len(ordered))
	for _, currency := range ordered {
		beforeValue, afterValue := beforeTotals[currency], afterTotals[currency]
		out = append(out, CurrencyDiffTotal{Currency: currency, Before: beforeValue, After: afterValue, Delta: afterValue - beforeValue})
	}
	return out
}

// ComputeDiffChecked pairs resources by address and computes monthly deltas,
// rejecting any non-finite component or aggregate before it can reach JSON.
func ComputeDiffChecked(before, after Report) (DiffReport, error) {
	if err := ValidateFiniteReport(before); err != nil {
		return DiffReport{}, fmt.Errorf("before report: %w", err)
	}
	if err := ValidateFiniteReport(after); err != nil {
		return DiffReport{}, fmt.Errorf("after report: %w", err)
	}
	return computeDiffUnchecked(before, after), nil
}

// ComputeDiff preserves the original convenience API. Invalid externally-built
// reports produce an explicit aggregation error rather than non-finite numbers.
func ComputeDiff(before, after Report) DiffReport {
	report, err := ComputeDiffChecked(before, after)
	if err != nil {
		return DiffReport{AggregationError: err.Error(), TotalsComplete: false}
	}
	return report
}

func computeDiffUnchecked(before, after Report) DiffReport {
	idx := func(rep Report) map[string]ResourceCost {
		m := make(map[string]ResourceCost, len(rep.Resources))
		for _, r := range rep.Resources {
			m[r.Address] = r
		}
		return m
	}
	bm, am := idx(before), idx(after)
	beforeSkipped := make(map[string]bool, len(before.Skipped))
	afterSkipped := make(map[string]bool, len(after.Skipped))
	for _, skipped := range before.Skipped {
		beforeSkipped[skipped.Address] = true
	}
	for _, skipped := range after.Skipped {
		afterSkipped[skipped.Address] = true
	}
	seen := map[string]bool{}
	out := DiffReport{TotalsComplete: len(before.Skipped) == 0 && len(after.Skipped) == 0}

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
		bTotal, bCurrency, bComparable := resourceMonthlyCurrency(br)
		if ar, ok := am[addr]; ok {
			aTotal, aCurrency, aComparable := resourceMonthlyCurrency(ar)
			if !bComparable || !aComparable || bCurrency != aCurrency {
				out.Resources = append(out.Resources, ResourceDiff{
					Address: addr, Type: br.Type, Kind: DiffChange,
					BeforeMonthly: bTotal, AfterMonthly: aTotal, Uncertain: true,
				})
				continue
			}
			kind := DiffSame
			if math.Abs(aTotal-bTotal) > 1e-6 {
				kind = DiffChange
			}
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: br.Type, Kind: kind,
				BeforeMonthly: bTotal, AfterMonthly: aTotal, DeltaMonthly: aTotal - bTotal,
			})
		} else if afterSkipped[addr] {
			// The resource still exists on the after side but could not be priced.
			// Do not misreport it as a removal or claim a numeric saving from zero.
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: br.Type, Kind: DiffChange,
				BeforeMonthly: bTotal, Uncertain: true,
			})
		} else if !bComparable {
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: br.Type, Kind: DiffChange,
				BeforeMonthly: bTotal, Uncertain: true,
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
		aTotal, _, aComparable := resourceMonthlyCurrency(ar)
		if beforeSkipped[addr] || !aComparable {
			// The resource existed before but was unpriced. Its delta is unknown,
			// not an addition from a known zero baseline.
			out.Resources = append(out.Resources, ResourceDiff{
				Address: addr, Type: ar.Type, Kind: DiffChange,
				AfterMonthly: aTotal, Uncertain: true,
			})
			continue
		}
		out.Resources = append(out.Resources, ResourceDiff{
			Address: addr, Type: ar.Type, Kind: DiffAdd,
			BeforeMonthly: 0, AfterMonthly: aTotal, DeltaMonthly: aTotal,
		})
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		return out.Resources[i].Address < out.Resources[j].Address
	})
	out.TotalsByCurrency = currencyDiffTotals(before, after)
	if out.Currency != "" {
		out.BeforeTotal = before.Total()
		out.AfterTotal = after.Total()
		out.DeltaTotal = out.AfterTotal - out.BeforeTotal
	}

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
	if d.AggregationError != "" {
		return fmt.Errorf("cannot render diff: %s", d.AggregationError)
	}
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(d)
	case "markdown":
		return renderDiffMarkdown(w, d)
	case "table", "":
		return renderDiffTable(w, d)
	}
	return fmt.Errorf("unknown format %q", format)
}

func diffTotalsComplete(d DiffReport) bool {
	if d.TotalsComplete {
		return true
	}
	if len(d.Skipped) > 0 {
		return false
	}
	for _, resource := range d.Resources {
		if resource.Uncertain {
			return false
		}
	}
	// Preserve the behavior of callers that construct DiffReport literals and
	// predate the additive TotalsComplete field.
	return true
}

func renderDiffTable(w io.Writer, d DiffReport) error {
	t := tablewriter.NewWriter(w)
	t.SetHeader([]string{"", "Address", "Type", "Before", "After", "Δ Monthly"})
	for _, r := range d.Resources {
		before, after, delta := fmt.Sprintf("%.2f", r.BeforeMonthly), fmt.Sprintf("%.2f", r.AfterMonthly), fmt.Sprintf("%+.2f", r.DeltaMonthly)
		if r.Uncertain {
			before, after, delta = "-", "-", "-"
		}
		t.Append([]string{string(r.Kind), r.Address, r.Type, before, after, delta})
	}
	// A skipped resource makes the aggregate delta incomplete even if all priced
	// components use one currency.
	if !diffTotalsComplete(d) {
		t.SetFooter([]string{"", "PRICED SUBTOTAL (incomplete)", "", "-", "-", "-"})
	} else if d.Currency == "" {
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
	// Only surface a single summed total when every resource was priced and every
	// component shares one currency. Missing usage or another skipped resource
	// makes the aggregate change unknown, not zero.
	if !diffTotalsComplete(d) {
		fmt.Fprintln(w, "**Monthly change:** _(incomplete — one or more resources were not priced)_")
		fmt.Fprintln(w)
	} else if d.Currency == "" {
		fmt.Fprintln(w, "**Monthly change:** _(mixed currencies — totals not summed)_")
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "**Monthly change:** `%+.2f %s` (before `%.2f` → after `%.2f`)\n\n",
			d.DeltaTotal, d.Currency, d.BeforeTotal, d.AfterTotal)
	}
	fmt.Fprintln(w, "|  | Resource | Before | After | Δ Monthly |")
	fmt.Fprintln(w, "|---|---|---:|---:|---:|")
	for _, r := range d.Resources {
		if r.Kind == DiffSame {
			continue
		}
		if r.Uncertain {
			fmt.Fprintf(w, "| %s | `%s` | — | — | **unknown** |\n", r.Kind, r.Address)
			continue
		}
		fmt.Fprintf(w, "| %s | `%s` | %.2f | %.2f | **%+.2f** |\n",
			r.Kind, r.Address, r.BeforeMonthly, r.AfterMonthly, r.DeltaMonthly)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> Priced via provider APIs or explicit, source-attributed user rates; unpriced usage is never assumed to be zero.")
	if len(d.Skipped) > 0 {
		// Group skipped addresses by their real reason (unsupported type, auth
		// failure, API error, parse failure, panic, ...) so the PR comment never
		// mislabels a transient error as "unsupported type".
		byReason := map[string][]string{}
		reasons := []string{}
		for _, s := range d.Skipped {
			reason := s.Reason
			if reason == "" {
				reason = "unknown"
			}
			if _, ok := byReason[reason]; !ok {
				reasons = append(reasons, reason)
			}
			byReason[reason] = append(byReason[reason], s.Address)
		}
		sort.Strings(reasons)
		fmt.Fprintf(w, "\n> ⚠️ %d resource(s) skipped:\n", len(d.Skipped))
		for _, reason := range reasons {
			addrs := byReason[reason]
			sort.Strings(addrs)
			escaped := make([]string, len(addrs))
			for i, a := range addrs {
				escaped[i] = mdEscape(a)
			}
			fmt.Fprintf(w, ">   - **%s** (%d): `%s`\n", mdEscape(reason), len(addrs), strings.Join(escaped, "`, `"))
		}
	}
	return nil
}

// mdEscape escapes the markdown-significant characters in s so that a skip
// reason or resource address containing "*", "_", "`", or "[" does not break
// the rendered PR comment. It does not escape the surrounding backticks we add
// ourselves when listing addresses.
func mdEscape(s string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
	).Replace(s)
}
