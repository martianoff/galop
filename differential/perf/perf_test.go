package perf

import "testing"

func TestJudge(t *testing.T) {
	b := Baseline{
		Report: Report{
			Counters: map[string]int64{
				"native/corpus/predictions":     100,
				"native/corpus/allocs-per-pass": 1000,
			},
			Ratios: map[string]float64{
				"corpus/w1/warm/galop-vs-antlr":  0.10,
				"corpus/w32/warm/galop-vs-lexer": 5,
			},
		},
		Rules: []Rule{
			{Pattern: "*/*/allocs-per-pass", Tolerance: 0.01},
			{Pattern: "*/w32/*/galop-vs-lexer", ReportOnly: true},
			{Pattern: "*/*/*/*", Tolerance: 0.3},
		},
	}
	cases := []struct {
		name    string
		r       Report
		verdict string
	}{
		{"the same count is ok", Report{Counters: map[string]int64{"native/corpus/predictions": 100}}, "ok"},
		{"one more prediction fails", Report{Counters: map[string]int64{"native/corpus/predictions": 101}}, "worse"},
		{"fewer is better", Report{Counters: map[string]int64{"native/corpus/predictions": 0}}, "better"},
		{"allocations within their tolerance", Report{Counters: map[string]int64{"native/corpus/allocs-per-pass": 1005}}, "ok"},
		{"allocations past their tolerance", Report{Counters: map[string]int64{"native/corpus/allocs-per-pass": 1011}}, "worse"},
		{"a ratio within the tolerance", Report{Ratios: map[string]float64{"corpus/w1/warm/galop-vs-antlr": 0.129}}, "ok"},
		{"a ratio past the tolerance", Report{Ratios: map[string]float64{"corpus/w1/warm/galop-vs-antlr": 0.131}}, "worse"},
		{"a report-only ratio never fails", Report{Ratios: map[string]float64{"corpus/w32/warm/galop-vs-lexer": 50}}, "reported"},
		{"a number the baseline lacks is new", Report{Counters: map[string]int64{"native/generated/predictions": 5}}, "new"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := Judge(c.r, b)
			if len(fs) != 1 || fs[0].Verdict != c.verdict {
				t.Fatalf("findings %+v, want one with verdict %q", fs, c.verdict)
			}
			if Failed(fs) != (c.verdict == "worse") {
				t.Errorf("Failed = %v", Failed(fs))
			}
		})
	}
}
