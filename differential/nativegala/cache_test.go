package nativegala

import (
	"math/rand"
	"sync"
	"testing"
)

// parseResult is what a parse gives, as text: the tree or the error.
func parseResult(src string) string {
	t, err := Parse(src)
	if err != nil {
		return "error: " + err.Error()
	}
	return t.ToStringTree()
}

// TestSLLCacheBounded parses many distinct inputs, mostly broken ones that
// lean on runtime prediction, under a small cache bound, from several
// goroutines at once. Every generation of the cache the test sees, replaced
// ones included, must stay within the bound; the cache must start over many
// times; and every input must give the result it gets with the default
// bound. It changes the process's cache, so it must not run in parallel with
// other tests; it restores the cache when done.
func TestSLLCacheBounded(t *testing.T) {
	files, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(1))
	var inputs []string
	for _, f := range files {
		toks := Tokens(f.Src).Tokens
		if len(f.Src) > 20000 || len(toks) < 3 {
			continue
		}
		inputs = append(inputs, f.Src)
		for range 2 {
			// Drop a token, or a span of a few; the last token is EOF.
			i := r.Intn(len(toks) - 1)
			j := min(i+1+r.Intn(3), len(toks)-1)
			inputs = append(inputs, f.Src[:toks[i].Start]+f.Src[toks[j].Start:])
		}
	}
	want := make([]string, len(inputs))
	for i, src := range inputs {
		want[i] = parseResult(src)
	}

	const bound, contexts = 200, 1 << 14
	galaGalopDFA.mu.Lock()
	saved := struct {
		gen                    *galaGalopDFAGen
		peak, resets           int
		maxStates, maxContexts int
	}{galaGalopDFA.gen.Load(), galaGalopDFA.peak, galaGalopDFA.resets, galaGalopDFAMaxStates, galaGalopDFAMaxContexts}
	galaGalopDFAMaxStates, galaGalopDFAMaxContexts = bound, contexts
	galaGalopDFA.gen.Store(galaGalopNewDFAGen())
	galaGalopDFA.peak, galaGalopDFA.resets = 0, 0
	galaGalopDFA.mu.Unlock()
	defer func() {
		galaGalopDFA.mu.Lock()
		galaGalopDFA.gen.Store(saved.gen)
		galaGalopDFA.peak, galaGalopDFA.resets = saved.peak, saved.resets
		galaGalopDFAMaxStates, galaGalopDFAMaxContexts = saved.maxStates, saved.maxContexts
		galaGalopDFA.mu.Unlock()
	}()

	// Workers note each generation they see after a parse, so that replaced
	// generations can be checked once every prediction is done with them.
	var mu sync.Mutex
	seen := map[*galaGalopDFAGen]bool{}
	var wg sync.WaitGroup
	next := make(chan int)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				got := parseResult(inputs[i])
				g := galaGalopDFA.gen.Load()
				mu.Lock()
				seen[g] = true
				if got != want[i] {
					t.Errorf("input %d: the result changed with a bounded cache:\n got: %.200s\nwant: %.200s", i, got, want[i])
				}
				mu.Unlock()
			}
		}()
	}
	for i := range inputs {
		next <- i
	}
	close(next)
	wg.Wait()

	galaGalopDFA.mu.Lock()
	defer galaGalopDFA.mu.Unlock()
	for g := range seen {
		if n, c := len(g.states), len(g.pr.pool.kind); n > bound {
			t.Errorf("a generation of the cache holds %d states (%d contexts), past its bound of %d", n, c, bound)
		}
	}
	if galaGalopDFA.resets < 10 || len(seen) < 10 {
		t.Fatalf("the cache started over %d times, and the test saw %d generations; the bound was not exercised",
			galaGalopDFA.resets, len(seen))
	}
	if galaGalopDFA.peak > bound {
		t.Errorf("the peak is %d states, past the bound of %d", galaGalopDFA.peak, bound)
	}
	t.Logf("%d inputs: %d resets, %d generations checked, peak %d states", len(inputs), galaGalopDFA.resets, len(seen), galaGalopDFA.peak)
}
