// Command perfgate measures galop's generated parsers for gala.g4 and judges
// the numbers against a checked-in baseline (see docs/FUZZ_AND_PERF.md).
//
// It measures three inputs: a fixed subset of the GALA corpus, sentences
// galop-sample generated (valid code no corpus has), and mutated corpus files
// (mostly invalid code). For each it records
//
//   - deterministic counters, each in a fresh process so the process-wide
//     prediction caches start empty: runtime (adaptive) predictions,
//     full-context predictions, files that needed more than one pass and
//     their extra passes (median, max), the SLL DFA and full-context cache
//     states built, and allocations per warm pass; for both flavors;
//   - wall-clock times of the native flavor, ANTLR (the GALA compiler's
//     SLL-then-LL strategy) and the native lexer alone: warm (median of
//     -passes) and first pass in a fresh process (median of -runs), on 1 and
//     32 goroutines, and from them the ratios galop over ANTLR and galop over
//     the lexer. Absolute times are only reported;
//   - the warm time of converting ANTLR trees to the native tree (see
//     GalaToNative), only reported.
//
// The baseline file's rules say how far each number may move and which are
// only reported; a number the baseline has and the run lacks fails. Without
// -enforce the gate only reports. -update writes the run's numbers as the new
// baseline, keeping its rules (and its ratios when -times=false).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/measure"
	"github.com/martianoff/galop/differential/mutate"
	"github.com/martianoff/galop/differential/nativegala"
	"github.com/martianoff/galop/differential/perf"
)

var (
	manifest    = flag.String("manifest", "", "corpus_manifest of the GALA corpus")
	samples     = flag.String("samples", "", "galop-sample output: the generated input")
	baseline    = flag.String("baseline", "", "baseline file, by runfiles location or path")
	update      = flag.String("update", "", "write this run's numbers as the baseline to this path (relative to the workspace)")
	jsonOut     = flag.String("json", "", "also write this run's numbers to this file (CI keeps it as an artifact)")
	enforce     = flag.Bool("enforce", false, "exit 1 when a number regresses (default: report only)")
	every       = flag.Int("every", 3, "use every Nth corpus file")
	mutantCount = flag.Int("mutants", 100, "how many mutated corpus files to measure")
	runs        = flag.Int("runs", 5, "fresh processes per first-pass time, one per round (the median is kept)")
	passes      = flag.Int("passes", 5, "warm samples per time, one per round (the median is kept)")
	times       = flag.Bool("times", true, "measure wall-clock times (false: counters only)")
	child       = flag.String("child", "", "internal: run one measurement in this fresh process (counters:<flavor> or time:<phase>:<workers>)")
	inputFile   = flag.String("input_file", "", "internal, with -child: the input to measure, in galop-sample's format")
)

// inputNames are the measured inputs.
var inputNames = []string{"corpus", "generated", "mutants"}

// flavors are galop's two output flavors, as the counters name them.
var flavors = []string{"native", "antlrflavor"}

// defaultRules judge a baseline written without rules: counters exactly but
// allocations within 1% (the Go runtime's own background allocations move
// them by a few), and ratios on one goroutine within 30%. Ratios on 32
// goroutines are only reported: on the CI runner's four cores two runs of the
// same code differ by about a third. The input's size and how much of it
// parses are reported, not judged: the parity tests fail on any difference
// from ANTLR in what parses.
var defaultRules = []perf.Rule{
	{Pattern: "*/*/files", ReportOnly: true},
	{Pattern: "*/*/accepted", ReportOnly: true},
	{Pattern: "*/*/allocs-per-pass", Tolerance: 0.01},
	{Pattern: "*/w32/*/*", ReportOnly: true},
	{Pattern: "*/*/*/*", Tolerance: 0.3},
}

func main() {
	flag.Parse()
	os.Exit(run())
}

// run measures and reports, and returns the exit code: 1 when -enforce is set
// and a number regressed.
func run() int {
	if *child != "" {
		files, err := corpus.FromSamples(*inputFile)
		check(err)
		srcs := make([]string, len(files))
		for i, f := range files {
			srcs[i] = f.Src
		}
		runChild(srcs)
		return 0
	}
	inputs, err := loadInputs()
	check(err)
	// Children read their input from a file written once here, rather than
	// each loading the corpus and making the mutants again.
	dir, err := os.MkdirTemp("", "perfgate-")
	check(err)
	defer os.RemoveAll(dir)
	files := map[string]string{}
	for _, in := range inputNames {
		files[in] = filepath.Join(dir, in+".txt")
		check(corpus.WriteSamples(files[in], inputs[in]))
	}
	r := perf.Report{Counters: measureCounters(files), Ratios: map[string]float64{}, Info: map[string]float64{}}
	if *times {
		for _, in := range inputNames {
			measureTimes(r, inputs[in], in, files[in])
			measureConversion(r, inputs[in], in)
		}
	}
	return report(r)
}

// measureCounters runs the counter children, each in its own fresh process,
// all at once: counts do not depend on timing.
func measureCounters(files map[string]string) map[string]int64 {
	counters := map[string]int64{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, in := range inputNames {
		for _, flavor := range flavors {
			wg.Go(func() {
				var c map[string]int64
				check(measure.Child(&c, "-child=counters:"+flavor, "-input_file="+files[in]))
				mu.Lock()
				defer mu.Unlock()
				for k, v := range c {
					counters[flavor+"/"+in+"/"+k] = v
				}
			})
		}
	}
	wg.Wait()
	return counters
}

// timedPhases are the phases measureTimes times, in the order of each round.
var timedPhases = []string{"native", "antlr", "lexer"}

// measureTimes times srcs, the input called name (written to file for the
// children), and adds the times and ratios to r. The phases are timed in
// rounds, one sample of each per round, and a ratio to ANTLR is the median
// over the rounds of that round's ratio, so a slow stretch of a shared machine
// slows both sides alike.
func measureTimes(r perf.Report, srcs []string, name, file string) {
	for _, workers := range []int{1, 32} {
		reps := map[string]int{}
		for _, phase := range timedPhases {
			reps[phase] = calibrate(srcs, phases[phase], workers)
		}
		warmMS, firstMS := map[string][]float64{}, map[string][]float64{}
		for range *passes {
			for _, phase := range timedPhases {
				warmMS[phase] = append(warmMS[phase], warm(srcs, phases[phase], workers, reps[phase]))
			}
		}
		for range *runs {
			for _, phase := range timedPhases {
				firstMS[phase] = append(firstMS[phase], first(phase, workers, file))
			}
		}
		for _, phase := range timedPhases {
			w, f := median(warmMS[phase]), median(firstMS[phase])
			key := fmt.Sprintf("%s/w%d/%s/", name, workers, phase)
			r.Info[key+"warm-ms"], r.Info[key+"first-ms"], r.Info[key+"gap-ms"] = w, f, f-w
		}
		// The lexer's warm time is the floor every parse pays; its first pass
		// is too short to time well, so both of galop's times are compared
		// with the warm one.
		lexer := median(warmMS["lexer"])
		for when, ms := range map[string]map[string][]float64{"warm": warmMS, "first": firstMS} {
			key := fmt.Sprintf("%s/w%d/%s/", name, workers, when)
			r.Ratios[key+"galop-vs-antlr"] = medianRatio(ms["native"], ms["antlr"])
			r.Ratios[key+"galop-vs-lexer"] = median(ms["native"]) / lexer
		}
	}
}

// medianRatio is the median over rounds of a[i] / b[i].
func medianRatio(a, b []float64) float64 {
	ratios := make([]float64, len(a))
	for i := range a {
		ratios[i] = a[i] / b[i]
	}
	return median(ratios)
}

// report judges r, prints it (and adds it to the GitHub job summary),
// writes the baseline with -update, and returns the exit code.
func report(r perf.Report) int {
	b := perf.Baseline{Rules: defaultRules}
	if *baseline != "" {
		var err error
		b, err = perf.ReadBaseline(corpus.Locate(*baseline))
		check(err)
	}
	judged := b
	if !*times {
		// A counters-only run has no ratios: it doesn't judge them, and
		// keeps the baseline's when it writes one.
		judged.Ratios = nil
	}
	findings := perf.Judge(r, judged)
	md := fmt.Sprintf("## galop performance gate (%s)\n\nGOMAXPROCS=%d, %s/%s, %s\n\n%s",
		map[bool]string{true: "enforcing", false: "report only"}[*enforce],
		runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH, runtime.Version(), perf.Markdown(findings, r))
	fmt.Println(md)
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		if f, err := os.OpenFile(summary, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
			fmt.Fprintln(f, md)
			f.Close()
		}
	}
	if *jsonOut != "" {
		data, err := json.MarshalIndent(r, "", "  ")
		check(err)
		check(os.WriteFile(*jsonOut, append(data, '\n'), 0o644))
	}
	if *update != "" {
		path := *update
		if ws := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); ws != "" && !filepath.IsAbs(path) {
			path = filepath.Join(ws, path)
		}
		rules := b.Rules
		if len(rules) == 0 {
			rules = defaultRules
		}
		written := r
		note := fmt.Sprintf("measured by perfgate on %s/%s, GOMAXPROCS=%d", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
		if !*times {
			written.Ratios = b.Ratios
			note = fmt.Sprintf("counters %s; ratios kept from the previous baseline (%s)", note, b.Note)
		}
		check(perf.WriteBaseline(path, written, rules, note))
		fmt.Println("wrote", path)
	}
	if perf.Failed(findings) {
		if *enforce {
			fmt.Println("perfgate: a number regressed or is missing")
			return 1
		}
		fmt.Println("perfgate: a number regressed or is missing (report only)")
	}
	return 0
}

// phases are the timed operations.
var phases = map[string]func(string){
	"native": func(src string) { nativegala.Parse(src) },
	"antlr":  func(src string) { gala.Lenient(gala.Tokens(src)) },
	"lexer":  func(src string) { nativegala.Tokens(src) },
}

// runChild runs one measurement in this fresh process and prints it as JSON.
func runChild(srcs []string) {
	kind, rest, _ := strings.Cut(*child, ":")
	switch kind {
	case "counters":
		check(json.NewEncoder(os.Stdout).Encode(counters(rest, srcs)))
	case "time":
		phase, w, _ := strings.Cut(rest, ":")
		var workers int
		fmt.Sscan(w, &workers)
		elapsed := measure.Once(func() { measure.Pass(srcs, phases[phase], workers) }).Elapsed
		check(json.NewEncoder(os.Stdout).Encode(ms(elapsed)))
	}
}

// counters parses srcs once with a flavor, in a fresh process, and counts its
// work; then one more pass on the warm caches counts the allocations.
func counters(flavor string, srcs []string) map[string]int64 {
	parse := func(src string) (nativegala.Stats, bool) {
		_, s, err := nativegala.ParseWithStats(src)
		return s, err == nil
	}
	cacheStates, sllCache := nativegala.CacheStates, nativegala.SLLCache
	if flavor == "antlrflavor" {
		parse = func(src string) (nativegala.Stats, bool) {
			_, s, err := antlrgala.ParseGalopWithStats(gala.GalopTokens(src))
			return nativegala.Stats(s), err == nil
		}
		cacheStates, sllCache = antlrgala.CacheStates, antlrgala.SLLCache
	}
	c := map[string]int64{"files": int64(len(srcs)), "replays-median": 0, "replays-max": 0}
	var replays []int
	for _, src := range srcs {
		s, ok := parse(src)
		if ok {
			c["accepted"]++
		}
		c["predictions"] += int64(s.Predictions)
		c["full-context"] += int64(s.FullContext)
		if s.Passes > 1 {
			replays = append(replays, s.Passes-1)
		}
	}
	c["replayed-files"] = int64(len(replays))
	if len(replays) > 0 {
		slices.Sort(replays)
		c["replays-median"] = int64(replays[len(replays)/2])
		c["replays-max"] = int64(replays[len(replays)-1])
	}
	sll, full := cacheStates()
	c["sll-states"], c["full-context-states"] = int64(sll), int64(full)
	peak, resets := sllCache()
	c["sll-peak-states"], c["sll-resets"] = int64(peak), int64(resets)
	c["allocs-per-pass"] = int64(measure.Once(func() {
		for _, src := range srcs {
			parse(src)
		}
	}).Allocs)
	return c
}

// minSample is the shortest warm sample: shorter ones are mostly timer and
// scheduling noise.
const minSample = 50 * time.Millisecond

// calibrate warms the caches with two untimed passes, then times a warm one
// and returns how many passes make a sample last at least minSample.
func calibrate(srcs []string, do func(string), workers int) int {
	for range 2 {
		measure.Pass(srcs, do, workers)
	}
	one := max(measure.Once(func() { measure.Pass(srcs, do, workers) }).Elapsed, time.Microsecond)
	return int((minSample + one - 1) / one)
}

// warm is one warm pass in ms: passes run in batches of reps until they have
// lasted at least minSample, and their time is averaged.
func warm(srcs []string, do func(string), workers, reps int) float64 {
	start := time.Now()
	n := 0
	for n == 0 || time.Since(start) < minSample {
		for range reps {
			measure.Pass(srcs, do, workers)
		}
		n += reps
	}
	return ms(time.Since(start)) / float64(n)
}

// first is the first pass of a fresh process over the input in file, in ms.
func first(phase string, workers int, file string) float64 {
	var v float64
	check(measure.Child(&v, fmt.Sprintf("-child=time:%s:%d", phase, workers), "-input_file="+file))
	return v
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[len(s)/2]
}

// loadInputs reads the three inputs. The corpus subset and the mutants are
// fixed by -every, -mutants and the mutator's seed, so every run measures the
// same text.
func loadInputs() (map[string][]string, error) {
	files, err := corpus.Source{Manifest: *manifest}.Load()
	if err != nil {
		return nil, err
	}
	var subset []string
	for i, f := range files {
		if i%*every == 0 {
			subset = append(subset, gala.StripBOM(f.Src))
		}
	}
	gen, err := corpus.FromSamples(*samples)
	if err != nil {
		return nil, err
	}
	generated := make([]string, len(gen))
	for i, f := range gen {
		generated[i] = f.Src
	}
	inputs := map[string][]string{"corpus": subset, "generated": generated, "mutants": mutants(subset, *mutantCount)}
	for in, srcs := range inputs {
		if len(srcs) == 0 {
			return nil, fmt.Errorf("input %s is empty: check -every and -mutants", in)
		}
	}
	return inputs, nil
}

// mutants makes n mutants of the smaller sources with the diff tests'
// mutator, from a fixed seed.
func mutants(srcs []string, n int) []string {
	var small []string
	for _, s := range srcs {
		if len(s) < 20000 {
			small = append(small, s)
		}
	}
	if len(small) == 0 {
		return nil
	}
	r := rand.New(rand.NewSource(1))
	out := make([]string, n)
	for i := range out {
		out[i] = mutate.Mutate(r, small[r.Intn(len(small))])
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "perfgate:", err)
		os.Exit(2)
	}
}

// measureConversion adds the warm time of converting the input's ANTLR trees
// to the native tree, on one goroutine, to r's Info: galop's antlr flavor's
// tree where it accepts the file, ANTLR's error-recovered one where it does
// not. It is reported, not judged.
func measureConversion(r perf.Report, srcs []string, name string) {
	type parsed struct {
		root   antlr.ParserRuleContext
		stream *antlr.CommonTokenStream
		src    string
	}
	trees := make([]parsed, len(srcs))
	for i, src := range srcs {
		src = gala.StripBOM(src)
		stream := gala.GalopTokens(src)
		var root antlr.ParserRuleContext
		root, err := antlrgala.NewgalaGalopParser(stream).SourceFile()
		if err != nil {
			stream = gala.Tokens(src)
			root, _ = gala.Lenient(stream)
		}
		trees[i] = parsed{root, stream, src}
	}
	// One pass over the single input "" converts every tree, so the
	// conversion is sampled as the other phases are.
	all := []string{""}
	convert := func(string) {
		for _, t := range trees {
			antlrgala.GalaToNative(t.root, t.stream, t.src)
		}
	}
	reps := calibrate(all, convert, 1)
	var samples []float64
	for range *passes {
		samples = append(samples, warm(all, convert, 1, reps))
	}
	r.Info[name+"/w1/convert/warm-ms"] = median(samples)
}
