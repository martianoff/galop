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
//     the lexer. Absolute times are only reported.
//
// The baseline file's rules say how far each number may move and which are
// only reported. Without -enforce the gate only reports. -update writes the
// run's numbers as the new baseline, keeping its rules.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
	"github.com/martianoff/galop/differential/perf"
)

var (
	manifest  = flag.String("manifest", "", "corpus_manifest of the GALA corpus")
	samples   = flag.String("samples", "", "galop-sample output: the generated input")
	baseline  = flag.String("baseline", "", "baseline file, by runfiles location or path")
	update    = flag.String("update", "", "write this run's numbers as the baseline to this path (relative to the workspace)")
	jsonOut   = flag.String("json", "", "also write this run's numbers to this file (CI keeps it as an artifact)")
	enforce   = flag.Bool("enforce", false, "exit 1 when a number regresses (default: report only)")
	every     = flag.Int("every", 3, "use every Nth corpus file")
	mutants   = flag.Int("mutants", 100, "how many mutated corpus files to measure")
	runs      = flag.Int("runs", 5, "fresh processes per first-pass time, one per round (the median is kept)")
	passes    = flag.Int("passes", 5, "warm samples per time, one per round (the median is kept)")
	times     = flag.Bool("times", true, "measure wall-clock times (false: counters only)")
	child     = flag.String("child", "", "internal: run one measurement in this fresh process (counters:<flavor> or time:<phase>:<workers>)")
	inputFile = flag.String("input_file", "", "internal, with -child: the input to measure, in galop-sample's format")
)

// inputNames are the measured inputs.
var inputNames = []string{"corpus", "generated", "mutants"}

// flavors are galop's two output flavors, as the counters name them.
var flavors = []string{"native", "antlrflavor"}

// defaultRules judge a baseline written without rules: counters exactly but
// allocations within 1% (the Go runtime's own background allocations move
// them by a few), ratios within 30% (50% on 32 goroutines, which the CI
// runner's four cores make noisier), and galop over the lexer on 32
// goroutines only reported (a lexer pass there is well under a millisecond).
// The input's size and how much of it parses are reported, not judged: fewer
// accepted files is not "better", and the parity tests check acceptance.
var defaultRules = []perf.Rule{
	{Pattern: "*/*/files", ReportOnly: true},
	{Pattern: "*/*/accepted", ReportOnly: true},
	{Pattern: "*/*/allocs-per-pass", Tolerance: 0.01},
	{Pattern: "*/w32/*/galop-vs-lexer", ReportOnly: true},
	{Pattern: "*/w32/*/*", Tolerance: 0.5},
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
				check(json.Unmarshal(childRun("counters:"+flavor, files[in]), &c))
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
	findings := perf.Judge(r, b)
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
		note := fmt.Sprintf("measured by perfgate on %s/%s, GOMAXPROCS=%d", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
		check(perf.WriteBaseline(path, r, rules, note))
		fmt.Println("wrote", path)
	}
	if perf.Failed(findings) {
		if *enforce {
			fmt.Println("perfgate: a number regressed")
			return 1
		}
		fmt.Println("perfgate: a number regressed (report only)")
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
		start := time.Now()
		pass(srcs, phases[phase], workers)
		check(json.NewEncoder(os.Stdout).Encode(float64(time.Since(start).Microseconds()) / 1000))
	}
}

// counters parses srcs once with a flavor, in a fresh process, and counts its
// work; then one more pass on the warm caches counts the allocations.
func counters(flavor string, srcs []string) map[string]int64 {
	parse := func(src string) (nativegala.Stats, bool) {
		_, s, err := nativegala.ParseWithStats(src)
		return s, err == nil
	}
	cacheStates := nativegala.CacheStates
	if flavor == "antlrflavor" {
		parse = func(src string) (nativegala.Stats, bool) {
			_, s, err := antlrgala.ParseGalopWithStats(gala.GalopTokens(src))
			return nativegala.Stats(s), err == nil
		}
		cacheStates = antlrgala.CacheStates
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
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for _, src := range srcs {
		parse(src)
	}
	runtime.ReadMemStats(&after)
	c["allocs-per-pass"] = int64(after.Mallocs - before.Mallocs)
	return c
}

// calibrate runs one untimed pass and returns how many passes make a warm
// sample last at least 50 ms, so short passes (the lexer, many goroutines)
// time well too.
func calibrate(srcs []string, do func(string), workers int) int {
	start := time.Now()
	pass(srcs, do, workers)
	return max(1, int(50*time.Millisecond/max(time.Since(start), time.Microsecond)))
}

// warm is one warm pass in ms, averaged over reps passes.
func warm(srcs []string, do func(string), workers, reps int) float64 {
	start := time.Now()
	for range reps {
		pass(srcs, do, workers)
	}
	return float64(time.Since(start).Microseconds()) / 1000 / float64(reps)
}

// first is the first pass of a fresh process over the input in file, in ms.
func first(phase string, workers int, file string) float64 {
	var ms float64
	check(json.Unmarshal(childRun(fmt.Sprintf("time:%s:%d", phase, workers), file), &ms))
	return ms
}

// childRun runs this program again as a fresh process for one measurement of
// the input in file.
func childRun(what, file string) []byte {
	self, err := os.Executable()
	check(err)
	cmd := exec.Command(self, "-child="+what, "-input_file="+file)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	check(err)
	return out
}

// pass runs do over every source on workers goroutines.
func pass(srcs []string, do func(string), workers int) {
	if workers <= 1 {
		for _, s := range srcs {
			do(s)
		}
		return
	}
	next := make(chan string, len(srcs))
	for _, s := range srcs {
		next <- s
	}
	close(next)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for s := range next {
				do(s)
			}
		})
	}
	wg.Wait()
}

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
	inputs := map[string][]string{"corpus": subset, "generated": generated, "mutants": mutate(subset, *mutants)}
	for in, srcs := range inputs {
		if len(srcs) == 0 {
			return nil, fmt.Errorf("input %s is empty: check -every and -mutants", in)
		}
	}
	return inputs, nil
}

// insertions are the text mutate inserts: stray tokens and brackets.
var insertions = []string{"(", ")", "=>", "x", ".", ",", "=", "{", "}", "[", "]", "case (a, b) => ", "if x {", "Foo{a: 1}"}

// mutate makes n mutants of the smaller sources: a token dropped, doubled or
// preceded by stray text, at a position a fixed seed picks. It is a smaller
// cousin of the diff tests' mutator, which lives in a test package.
func mutate(srcs []string, n int) []string {
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
	out := make([]string, 0, n)
	for len(out) < n {
		src := small[r.Intn(len(small))]
		toks := nativegala.Tokens(src).Tokens
		if len(toks) < 2 {
			continue
		}
		tok := toks[r.Intn(len(toks)-1)]
		off, end := int(tok.Start), int(tok.End)
		switch r.Intn(3) {
		case 0:
			out = append(out, src[:off]+src[end:])
		case 1:
			out = append(out, src[:end]+" "+src[off:])
		default:
			out = append(out, src[:off]+insertions[r.Intn(len(insertions))]+" "+src[off:])
		}
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "perfgate:", err)
		os.Exit(2)
	}
}
