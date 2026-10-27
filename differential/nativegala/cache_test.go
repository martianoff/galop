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
// goroutines at once. The cache must stay within the bound, start over many
// times, and give every input the result it gets with the default bound.
func TestSLLCacheBounded(t *testing.T) {
	files, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(1))
	var inputs []string
	for _, f := range files {
		if len(f.Src) > 20000 {
			continue
		}
		inputs = append(inputs, f.Src)
		toks := Tokens(f.Src).Tokens
		for range 2 {
			// Drop a token, or a span of a few.
			i := r.Intn(len(toks) - 1)
			j := min(i+1+r.Intn(3), len(toks)-1)
			inputs = append(inputs, f.Src[:toks[i].Start]+f.Src[toks[j].Start:])
		}
	}
	want := make([]string, len(inputs))
	for i, src := range inputs {
		want[i] = parseResult(src)
	}

	maxStates, maxContexts := galaGalopDFAMaxStates, galaGalopDFAMaxContexts
	defer func() {
		galaGalopDFA.mu.Lock()
		galaGalopDFAMaxStates, galaGalopDFAMaxContexts = maxStates, maxContexts
		galaGalopDFA.gen.Store(galaGalopNewDFAGen())
		galaGalopDFA.mu.Unlock()
	}()
	const bound = 200
	galaGalopDFA.mu.Lock()
	galaGalopDFAMaxStates, galaGalopDFAMaxContexts = bound, 1<<14
	galaGalopDFA.gen.Store(galaGalopNewDFAGen())
	resetsBefore := galaGalopDFA.resets
	galaGalopDFA.mu.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	next := make(chan int)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if got := parseResult(inputs[i]); got != want[i] {
					mu.Lock()
					t.Errorf("input %d: the result changed with a bounded cache:\n got: %.200s\nwant: %.200s", i, got, want[i])
					mu.Unlock()
				}
			}
		}()
	}
	for i := range inputs {
		next <- i
		if i%64 == 0 {
			// A prediction extends the generation it started in, which may
			// already have been replaced; the current one stays bounded.
			if states, _, _ := galaGalopDFAStats(); states > bound {
				t.Errorf("after %d inputs the cache holds %d states, past its bound of %d", i, states, bound)
			}
		}
	}
	close(next)
	wg.Wait()
	states, peak, resets := galaGalopDFAStats()
	if states > bound {
		t.Errorf("the cache holds %d states, past its bound of %d", states, bound)
	}
	if resets == resetsBefore {
		t.Fatalf("the cache never started over (%d states, peak %d)", states, peak)
	}
	t.Logf("%d inputs: %d resets, peak %d states", len(inputs), resets-resetsBefore, peak)
}
