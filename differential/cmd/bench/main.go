// Command bench times ANTLR's lexer and parser for gala.g4 on a corpus.
//
// It measures two phases, lex (the token stream a parse reads, GALA's
// re-typing included) and parse (lex plus the GALA compiler's SLL-then-LL
// parse), each:
//
//   - warm: in this process, after one untimed pass has built ANTLR's DFA
//     caches, timed with testing.Benchmark; per file for the largest files,
//     and per corpus pass on one goroutine and on GOMAXPROCS goroutines;
//   - cold: the first pass in a fresh process, for which bench runs itself
//     as a subprocess (-child). The time is measured inside the child, so
//     process start-up is excluded; ANTLR's ATN deserialisation and DFA
//     construction are included.
//
// The output is a Markdown table.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
)

// phases are the measured operations, by name.
var phases = map[string]func(src string){
	"lex":   func(src string) { gala.Tokens(src).Fill() },
	"parse": func(src string) { gala.Lenient(gala.Tokens(src)) },
}

var phaseOrder = []string{"lex", "parse"}

// sample is one measurement.
type sample struct {
	Elapsed time.Duration `json:"elapsed"`
	Allocs  uint64        `json:"allocs"`
	Bytes   int           `json:"bytes"`
}

func main() {
	var src corpus.Source
	src.RegisterFlags(flag.CommandLine)
	top := flag.Int("top", 5, "time the N largest files individually")
	coldRuns := flag.Int("cold", 3, "fresh processes per cold measurement (the best is reported); 0 skips cold runs")
	child := flag.String("child", "", "internal: time one cold pass of this phase over the input and print it as JSON")
	par := flag.Bool("parallel", false, "internal, with -child: spread the pass over GOMAXPROCS goroutines")
	flag.Parse()
	src.Files = flag.Args()

	files, err := src.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(2)
	}
	if *child != "" {
		s := measure(files, phases[*child], *par)
		if err := json.NewEncoder(os.Stdout).Encode(s); err != nil {
			fmt.Fprintln(os.Stderr, "bench:", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("%d files, %.2f MB, GOMAXPROCS=%d, %s/%s, %s\n\n", len(files),
		float64(corpus.Bytes(files))/1e6, runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Println("| What | Phase | Time | MB/s | Allocs |")
	fmt.Println("|---|---|---:|---:|---:|")

	// Warm everything up once so the warm rows do not pay for the DFA build.
	for _, name := range phaseOrder {
		pass(files, phases[name], true)
	}
	for _, f := range corpus.Largest(files, *top) {
		one := []corpus.File{f}
		for _, name := range phaseOrder {
			row(fmt.Sprintf("`%s` (%d lines), warm", f.Name, strings.Count(f.Src, "\n")), name,
				warm(one, phases[name], false))
		}
	}
	for _, name := range phaseOrder {
		row("corpus, warm, 1 goroutine", name, warm(files, phases[name], false))
	}
	for _, name := range phaseOrder {
		row("corpus, warm, parallel", name, warm(files, phases[name], true))
	}
	if *coldRuns == 0 {
		return
	}
	for _, f := range corpus.Largest(files, *top) {
		for _, name := range phaseOrder {
			row(fmt.Sprintf("`%s`, cold", f.Name), name, cold(*coldRuns, name, false, []string{abs(f.Path)}))
		}
	}
	all := make([]string, len(files))
	for i, f := range files {
		all[i] = abs(f.Path)
	}
	for _, name := range phaseOrder {
		row("corpus, cold, 1 goroutine", name, cold(*coldRuns, name, false, all))
	}
	for _, name := range phaseOrder {
		row("corpus, cold, parallel", name, cold(*coldRuns, name, true, all))
	}
}

// warm times passes of do over files with testing.Benchmark, which repeats
// them until the total is long enough to measure, and returns one pass.
func warm(files []corpus.File, do func(string), parallel bool) sample {
	r := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			pass(files, do, parallel)
		}
	})
	return sample{Elapsed: time.Duration(r.NsPerOp()), Allocs: uint64(r.AllocsPerOp()), Bytes: corpus.Bytes(files)}
}

// measure times a single pass of do over files, as a fresh process sees it.
// Allocations are counted process-wide, so they are exact only when nothing
// else runs.
func measure(files []corpus.File, do func(string), parallel bool) sample {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	pass(files, do, parallel)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	return sample{Elapsed: elapsed, Allocs: after.Mallocs - before.Mallocs, Bytes: corpus.Bytes(files)}
}

// pass runs do over every file, on one goroutine or on GOMAXPROCS.
func pass(files []corpus.File, do func(string), parallel bool) {
	if parallel {
		var wg sync.WaitGroup
		next := make(chan string)
		for range runtime.GOMAXPROCS(0) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for src := range next {
					do(src)
				}
			}()
		}
		for _, f := range files {
			next <- f.Src
		}
		close(next)
		wg.Wait()
	} else {
		for _, f := range files {
			do(f.Src)
		}
	}
}

// best runs m n times and keeps the fastest sample.
func best(n int, m func() sample) sample {
	samples := make([]sample, n)
	for i := range samples {
		samples[i] = m()
	}
	return slices.MinFunc(samples, func(a, b sample) int { return int(a.Elapsed - b.Elapsed) })
}

// cold runs the phase in n fresh child processes and keeps the fastest.
// The files are passed in a list file, as a corpus can exceed the command
// line limit.
func cold(n int, phase string, parallel bool, paths []string) sample {
	list, err := os.CreateTemp("", "bench-files-*")
	check(err)
	defer os.Remove(list.Name())
	_, err = list.WriteString(strings.Join(paths, "\n") + "\n")
	check(err)
	check(list.Close())
	self, err := os.Executable()
	check(err)
	return best(n, func() sample {
		cmd := exec.Command(self, "-child="+phase, fmt.Sprintf("-parallel=%t", parallel), "-list="+list.Name())
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		check(err)
		var s sample
		check(json.Unmarshal(out, &s))
		return s
	})
}

func row(what, phase string, s sample) {
	ms := float64(s.Elapsed) / float64(time.Millisecond)
	mbps := float64(s.Bytes) / 1e6 / s.Elapsed.Seconds()
	fmt.Printf("| %s | %s | %.2f ms | %.1f | %d |\n", what, phase, ms, mbps, s.Allocs)
}

// abs makes p independent of the working directory, which a child process
// may resolve differently under `bazel run`.
func abs(p string) string {
	a, err := filepath.Abs(p)
	check(err)
	return a
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}
