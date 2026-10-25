package diff

import (
	"flag"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
)

var manifest = flag.String("manifest", "", "corpus_manifest to read")

var loadCorpus = sync.OnceValues(func() ([]corpus.File, error) {
	return corpus.Source{Manifest: *manifest}.Load()
})

func files(tb testing.TB) []corpus.File {
	tb.Helper()
	fs, err := loadCorpus()
	if err != nil {
		tb.Fatal(err)
	}
	return fs
}

// rejected lists the corpus files gala.g4 does not accept, with the reason.
// The GALA compiler's own test suite skips the same files.
var rejected = map[string]string{
	"examples/slice_type_param.gala": "uses []T as a type argument, which gala.g4 has no syntax for",
}

// TestSLLMatchesLL compares ANTLR's two prediction modes on every corpus
// file. SLL may fail where LL succeeds (the GALA compiler then parses again
// with LL), but a tree SLL builds without a syntax error must be LL's tree:
// otherwise the SLL-first strategy would be a second reference that a
// generated parser has to match. The files SLL rejects are logged; they are
// the decisions that need full-context prediction.
func TestSLLMatchesLL(t *testing.T) {
	results := Run(files(t), gala.ANTLRParser(gala.SLL), gala.ANTLRParser(gala.LL))
	var silent, sllOnly []string
	for _, r := range results {
		switch {
		case r.Same:
		case r.ErrA == nil:
			silent = append(silent, r.File.Name+"\n  "+strings.ReplaceAll(r.Diff, "\n", "\n  "))
		default:
			sllOnly = append(sllOnly, r.File.Name)
		}
	}
	if len(silent) > 0 {
		t.Errorf("SLL built a different tree without a syntax error in %d files:\n%s",
			len(silent), strings.Join(silent, "\n"))
	}
	t.Logf("%d/%d identical; SLL reports syntax errors and builds a different tree in %d files:\n  %s",
		len(results)-len(silent)-len(sllOnly), len(results), len(sllOnly), strings.Join(sllOnly, "\n  "))
}

// TestLenientMatchesLL checks the GALA compiler's strategy, SLL with an LL
// retry, against LL alone: tree for tree, error-recovered trees included.
// That makes either one the reference a generated parser is held to.
func TestLenientMatchesLL(t *testing.T) {
	var out strings.Builder
	if n := Report(&out, "antlr-lenient", "antlr-ll", Run(files(t), gala.Lenient, gala.ANTLRParser(gala.LL))); n > 0 {
		t.Errorf("%d trees differ:\n%s", n, out.String())
	}
}

// TestGalopMatchesANTLR is the generated parser's acceptance test: on every
// corpus file it must build exactly the tree the GALA compiler gets from ANTLR
// (SLL with an LL retry), from the same tokens, and it must reject the files
// ANTLR rejects. Token parity of galop's lexer is lexdiff's job, so together
// the two cover the whole galop pipeline.
func TestGalopMatchesANTLR(t *testing.T) {
	galop, ok := gala.Parsers()["galop"]
	if !ok {
		t.Fatal("galop's antlr flavor is not registered")
	}
	var out strings.Builder
	var accepted []Result
	for _, r := range Run(files(t), galop, gala.Lenient) {
		if _, skip := rejected[r.File.Name]; skip {
			if r.ErrA == nil {
				t.Errorf("%s: galop accepts a file ANTLR rejects", r.File.Name)
			}
			continue
		}
		accepted = append(accepted, r)
	}
	if n := Report(&out, "galop", "antlr-lenient", accepted); n > 0 {
		t.Errorf("%d trees differ:\n%s", n, out.String())
	}
	t.Logf("%d/%d corpus files give ANTLR's tree", len(accepted), len(files(t)))
}

// TestCorpusParses checks that the reference parser accepts every corpus
// file under LL prediction except those listed in rejected: a file it
// rejects cannot tell a correct generated parser from a wrong one, and a
// rejected file that starts to parse means the list is out of date.
func TestCorpusParses(t *testing.T) {
	failing := map[string]bool{}
	for _, f := range files(t) {
		if _, errs := gala.ParseANTLR(f.Src, gala.LL); len(errs) > 0 {
			failing[f.Name] = true
			if _, ok := rejected[f.Name]; !ok {
				t.Errorf("%s: %v", f.Name, errs)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(rejected)) {
		if !failing[name] {
			t.Errorf("%s parses; remove it from rejected", name)
		}
	}
}

// BenchmarkLex lexes the whole corpus once per iteration (the token stream a
// parse reads, re-typing included). Each benchmark runs one untimed
// pass first, so it measures ANTLR with its DFA caches built.
func BenchmarkLex(b *testing.B) {
	fs := files(b)
	lexAll := func() {
		for _, f := range fs {
			gala.Tokens(f.Src).Fill()
		}
	}
	b.SetBytes(int64(corpus.Bytes(fs)))
	lexAll() // untimed: builds the lexer DFA
	for b.Loop() {
		lexAll()
	}
}

// BenchmarkParse lexes and parses the whole corpus once per iteration, the
// GALA compiler's way (SLL, LL on a syntax error).
func BenchmarkParse(b *testing.B) {
	fs := files(b)
	parseAll := func() {
		for _, f := range fs {
			gala.Lenient(gala.Tokens(f.Src))
		}
	}
	b.SetBytes(int64(corpus.Bytes(fs)))
	parseAll() // untimed: builds the DFAs
	for b.Loop() {
		parseAll()
	}
}

// BenchmarkParseGalop lexes and parses the whole corpus with galop's generated
// lexer and parser once per iteration.
func BenchmarkParseGalop(b *testing.B) {
	fs := files(b)
	galop := gala.Parsers()["galop"]
	parseAll := func() {
		for _, f := range fs {
			galop(gala.GalopTokens(f.Src))
		}
	}
	b.SetBytes(int64(corpus.Bytes(fs)))
	parseAll() // untimed: builds the cached SLL DFA states
	for b.Loop() {
		parseAll()
	}
}

// BenchmarkParseGalopParallel is BenchmarkParseGalop with files spread over
// GOMAXPROCS goroutines.
func BenchmarkParseGalopParallel(b *testing.B) {
	fs := files(b)
	galop := gala.Parsers()["galop"]
	b.SetBytes(int64(corpus.Bytes(fs)))
	parallel(fs, func(f corpus.File) { galop(gala.GalopTokens(f.Src)) })
	for b.Loop() {
		parallel(fs, func(f corpus.File) { galop(gala.GalopTokens(f.Src)) })
	}
}

// BenchmarkParseParallel is BenchmarkParse with files spread over
// GOMAXPROCS goroutines.
func BenchmarkParseParallel(b *testing.B) {
	fs := files(b)
	b.SetBytes(int64(corpus.Bytes(fs)))
	parallel(fs, func(f corpus.File) { gala.Lenient(gala.Tokens(f.Src)) }) // untimed: builds the DFAs
	for b.Loop() {
		parallel(fs, func(f corpus.File) { gala.Lenient(gala.Tokens(f.Src)) })
	}
}

func parallel(fs []corpus.File, do func(corpus.File)) {
	var wg sync.WaitGroup
	next := make(chan corpus.File)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range next {
				do(f)
			}
		}()
	}
	for _, f := range fs {
		next <- f
	}
	close(next)
	wg.Wait()
}
