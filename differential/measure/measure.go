// Package measure holds what the timing tools (cmd/bench, cmd/perfgate)
// share: running an operation over a corpus on several goroutines, timing
// one run with its allocations, and running a measurement in a fresh process.
package measure

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Pass runs do over every source, in order on this goroutine when workers is
// 1 or less, otherwise on workers goroutines.
func Pass(srcs []string, do func(string), workers int) { Passes(srcs, do, workers, 1) }

// Passes runs n passes of do over every source. On one worker they run in
// order on this goroutine. Otherwise workers goroutines share all n passes,
// taking sources in batches from one index (about eight batches each per
// pass, one source when there are fewer sources than that), so the work stays
// balanced and no goroutine waits for the others between passes. On
// Milvus's filter expressions, whose pass takes galop about a millisecond on
// 32 goroutines, starting the goroutines and waiting for the last of them
// each pass made it 40% longer.
func Passes(srcs []string, do func(string), workers, n int) {
	if workers <= 1 {
		for range n {
			for _, s := range srcs {
				do(s)
			}
		}
		return
	}
	total := n * len(srcs)
	batch := max(1, len(srcs)/(workers*8))
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				end := int(next.Add(int64(batch)))
				start := end - batch
				if start >= total {
					return
				}
				for i := start; i < min(end, total); i++ {
					do(srcs[i%len(srcs)])
				}
			}
		})
	}
	wg.Wait()
}

// Sample is one timed run.
type Sample struct {
	Elapsed time.Duration `json:"elapsed"`
	// Allocs and AllocMB are counted process-wide, so they are exact only
	// when nothing else runs.
	Allocs  uint64  `json:"allocs"`
	AllocMB float64 `json:"alloc_mb"`
}

// Once runs f once, after a garbage collection, and times it.
func Once(f func()) Sample {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	return Sample{Elapsed: elapsed, Allocs: after.Mallocs - before.Mallocs, AllocMB: float64(after.TotalAlloc-before.TotalAlloc) / 1e6}
}

// Child runs this program again with args, as a fresh process (its first
// pass sees empty caches), and decodes the JSON it prints into v. The child's
// standard error passes through.
func Child(v any, args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}
