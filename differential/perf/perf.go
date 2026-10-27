// Package perf holds the performance gate's numbers and the rules that judge
// them against a checked-in baseline (see docs/FUZZ_AND_PERF.md).
//
// Counters (runtime predictions, repair passes, cache states) are
// deterministic: the same parser on the same input counts the same, so by
// default a counter above its baseline fails the gate exactly. Wall-clock
// times are not: shared CI runners are noisy, so the gate judges ratios
// measured in one job (galop against ANTLR, and galop against the lexer
// alone), each with a tolerance over its baseline. Which number gets which
// tolerance, or is only reported, is policy, kept in the baseline file's
// rules rather than in code.
package perf

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
)

// Report is one run's measurements.
type Report struct {
	// Counters are named "flavor/input/counter", e.g. "native/corpus/predictions".
	Counters map[string]int64 `json:"counters"`
	// Ratios are named "input/workers/when/what", e.g.
	// "corpus/w1/warm/galop-vs-antlr": galop's time over the reference's, so
	// lower is better.
	Ratios map[string]float64 `json:"ratios"`
	// Info is reported and never judged: the median times behind the ratios,
	// in milliseconds, which depend on the machine.
	Info map[string]float64 `json:"info,omitempty"`
}

// Rule sets how the numbers whose names match Pattern are judged.
type Rule struct {
	// Pattern is a path.Match pattern over the number's name; "*" matches one
	// path element, e.g. "*/*/allocs-per-pass".
	Pattern string `json:"pattern"`
	// Tolerance is how far a matching number may move from its baseline
	// without a verdict: 0.3 allows 30%.
	Tolerance float64 `json:"tolerance"`
	// ReportOnly numbers are shown but never fail the gate.
	ReportOnly bool `json:"report_only,omitempty"`
}

// Baseline is the checked-in reference a run is judged against.
type Baseline struct {
	Report
	// Rules are tried in order; the first whose pattern matches a number's
	// name judges it. A number no rule matches has tolerance 0.
	Rules []Rule `json:"rules"`
	// Note says where the numbers come from.
	Note string `json:"note,omitempty"`
}

// rule is the rule that judges the number called name.
func (b Baseline) rule(name string) Rule {
	for _, r := range b.Rules {
		if ok, _ := path.Match(r.Pattern, name); ok {
			return r
		}
	}
	return Rule{Pattern: name}
}

// Finding is one number judged against its baseline.
type Finding struct {
	Name      string
	Got, Want float64
	// Kind is "counter" or "ratio".
	Kind string
	// Verdict is "ok", "better" (below the baseline by more than the
	// tolerance: tighten it), "worse" (a regression), "new" (not in the
	// baseline), "missing" (in the baseline but not measured: a renamed or
	// dropped number, which fails like a regression) or "reported" (a
	// report-only number).
	Verdict string
}

// Judge compares r with b: a number is worse when it exceeds its baseline by
// more than its rule's tolerance, and missing when b has it and r does not.
func Judge(r Report, b Baseline) []Finding {
	return append(judge(r.Counters, b.Counters, "counter", b), judge(r.Ratios, b.Ratios, "ratio", b)...)
}

func judge[V int64 | float64](got, want map[string]V, kind string, b Baseline) []Finding {
	var out []Finding
	for _, name := range slices.Sorted(maps.Keys(got)) {
		w, ok := want[name]
		f := Finding{Name: name, Got: float64(got[name]), Want: float64(w), Kind: kind}
		rule := b.rule(name)
		switch {
		case rule.ReportOnly:
			f.Verdict = "reported"
		case !ok:
			f.Verdict = "new"
		case f.Got > f.Want*(1+rule.Tolerance):
			f.Verdict = "worse"
		case f.Got < f.Want*(1-rule.Tolerance):
			f.Verdict = "better"
		default:
			f.Verdict = "ok"
		}
		out = append(out, f)
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if _, ok := got[name]; !ok && !b.rule(name).ReportOnly {
			out = append(out, Finding{Name: name, Want: float64(want[name]), Kind: kind, Verdict: "missing"})
		}
	}
	return out
}

// Failed reports whether any finding is a regression or missing.
func Failed(fs []Finding) bool {
	return slices.ContainsFunc(fs, func(f Finding) bool { return f.Verdict == "worse" || f.Verdict == "missing" })
}

var verdictText = map[string]string{
	"ok":       "ok",
	"better":   "better: tighten the baseline",
	"worse":    "**worse**",
	"new":      "new",
	"missing":  "**missing**",
	"reported": "reported only",
}

// Markdown renders the findings and r's Info as tables.
func Markdown(fs []Finding, r Report) string {
	var sb strings.Builder
	sb.WriteString("| Measure | Kind | Now | Baseline | Verdict |\n|---|---|---:|---:|---|\n")
	for _, f := range fs {
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s |\n", f.Name, f.Kind, number(f.Kind, f.Got), number(f.Kind, f.Want), verdictText[f.Verdict])
	}
	if len(r.Info) > 0 {
		sb.WriteString("\n| Reported, not judged | Value |\n|---|---:|\n")
		for _, name := range slices.Sorted(maps.Keys(r.Info)) {
			fmt.Fprintf(&sb, "| `%s` | %.4g |\n", name, r.Info[name])
		}
	}
	return sb.String()
}

func number(kind string, v float64) string {
	if kind == "counter" {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.3f", v)
}

// ReadBaseline reads a baseline file.
func ReadBaseline(path string) (Baseline, error) {
	var b Baseline
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// WriteBaseline writes r's counters and ratios as the baseline at path, with
// the given rules and note.
func WriteBaseline(path string, r Report, rules []Rule, note string) error {
	b := Baseline{Report: Report{Counters: r.Counters, Ratios: r.Ratios}, Rules: rules, Note: note}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
