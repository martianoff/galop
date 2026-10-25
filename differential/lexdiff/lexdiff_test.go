// Package lexdiff checks galop's generated lexer for gala.g4 against ANTLR's,
// token by token, and times the two.
//
//	bazel test //differential/lexdiff:lexdiff_test
//	bazel run -c opt //differential/lexdiff:lexdiff_test -- -test.run='^$' -test.bench=. -test.benchmem
package lexdiff

import (
	"flag"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/nativegala"
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

// lexed is everything a lexer run shows its caller: each token's fields, the
// input position and the line and column the lexer is left at after each
// token (GALA's line-break rule reads LA(1) there), and the errors reported.
type lexed struct {
	tokens []string
	errors []string
}

type recorder struct {
	*antlr.DefaultErrorListener
	errors *[]string
}

func (r recorder) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	*r.errors = append(*r.errors, fmt.Sprintf("%d:%d %s", line, column, msg))
}

// run reads every token of src, with the line and column seeded when line > 0,
// as the GALA compiler seeds them for an interpolated expression.
func run(lexer antlr.Lexer, base *antlr.BaseLexer, line, column int) lexed {
	var r lexed
	base.RemoveErrorListeners()
	base.AddErrorListener(recorder{errors: &r.errors})
	sim := base.Interpreter.(*antlr.LexerATNSimulator)
	if line > 0 {
		sim.Line, sim.CharPositionInLine = line, column
	}
	for {
		t := lexer.NextToken()
		in := lexer.GetInputStream()
		r.tokens = append(r.tokens, fmt.Sprintf("type=%d text=%q channel=%d %d..%d at %d:%d, then index=%d LA(1)=%d at %d:%d",
			t.GetTokenType(), t.GetText(), t.GetChannel(), t.GetStart(), t.GetStop(), t.GetLine(), t.GetColumn(),
			in.Index(), in.LA(1), sim.Line, sim.CharPositionInLine))
		if t.GetTokenType() == antlr.TokenEOF {
			return r
		}
	}
}

func antlrLex(src string, line, column int) lexed {
	l := antlrgala.NewgalaLexer(antlr.NewInputStream(src))
	return run(l, l.BaseLexer, line, column)
}

func galopLex(src string, line, column int) lexed {
	l := antlrgala.NewgalaGalopLexer(antlr.NewInputStream(src))
	return run(l, l.BaseLexer, line, column)
}

// same compares galop's antlr-flavor lexer with ANTLR's on src.
func same(t *testing.T, name, src string, line, column int) bool {
	t.Helper()
	return compare(t, name, antlrLex(src, line, column), "galop", galopLex(src, line, column))
}

// compare reports the first difference between ANTLR's run a and lexer
// other's run b.
func compare(t *testing.T, name string, a lexed, other string, b lexed) bool {
	t.Helper()
	if strings.Join(a.errors, "\n") != strings.Join(b.errors, "\n") {
		t.Errorf("%s: errors differ\nANTLR: %q\n%s: %q", name, a.errors, other, b.errors)
		return false
	}
	for i := 0; i < len(a.tokens) && i < len(b.tokens); i++ {
		if a.tokens[i] != b.tokens[i] {
			t.Errorf("%s: token %d differs\nANTLR: %s\n%s: %s", name, i, a.tokens[i], other, b.tokens[i])
			return false
		}
	}
	if len(a.tokens) != len(b.tokens) {
		t.Errorf("%s: %d tokens from ANTLR, %d from %s", name, len(a.tokens), len(b.tokens), other)
		return false
	}
	return true
}

// variants derives inputs from a file that reach the lexer's error and EOF
// paths: prefixes cut inside tokens, and characters inserted mid-file that
// open an unterminated token or match no rule.
func variants(src string) map[string]string {
	v := map[string]string{}
	r := []rune(src)
	for _, k := range []int{3, 7, 11, 13} {
		cut := len(r) * k / 17
		v[fmt.Sprintf("cut at %d", cut)] = string(r[:cut])
	}
	for _, ins := range []string{"§", "#", "\\", "'", "\"", "`", "s\"${", "/*", "@"} {
		mid := len(r) / 2
		v["insert "+ins] = string(r[:mid]) + ins + string(r[mid:])
	}
	return v
}

// TestCorpus lexes every corpus file (byte order mark included, which no rule
// matches) and its variants with both lexers.
func TestCorpus(t *testing.T) { corpusAndVariants(t, same) }

// corpusAndVariants runs check on every corpus file and its variants.
func corpusAndVariants(t *testing.T, check func(t *testing.T, name, src string, line, column int) bool) {
	fs := files(t)
	identical, variantsIdentical, variantsTotal := 0, 0, 0
	for _, f := range fs {
		if check(t, f.Name, f.Src, 0, 0) {
			identical++
		}
		for name, src := range variants(f.Src) {
			variantsTotal++
			if check(t, f.Name+" ("+name+")", src, 0, 0) {
				variantsIdentical++
			}
		}
	}
	t.Logf("%d/%d files identical; %d/%d variants identical", identical, len(fs), variantsIdentical, variantsTotal)
}

// cases are the inputs the corpus does not cover: empty input, interpolations
// nested past the static DFA's rule-call depth, odd endings.
func cases() map[string]string {
	nested := func(n int) string {
		return "val x = s\"a ${" + strings.Repeat("{ \"q\" ", n) + "1" + strings.Repeat(" }", n) + "} b\"\n"
	}
	cases := map[string]string{
		"empty":                "",
		"whitespace":           "  \n\t ",
		"nested 3":             nested(3),
		"nested 11":            nested(11),
		"nested 12":            nested(12),
		"nested 40":            nested(40),
		"nested unclosed":      "s\"${" + strings.Repeat("{", 30) + "x",
		"unterminated comment": "a /* b",
		"comment at end":       "x /* y */",
		"comment end twice":    "/* a */ b */",
		"non-ASCII":            "val é = \"ü\" // ß\nval 名 = 1",
		"CRLF":                 "a\r\nb\r\n",
		"backslash at end":     "\"a\\",
		"lone dollar":          "s\"$\"",
		"format string":        "f\"${x}%d ${\"a\"} $y\"",
	}
	return cases
}

// TestCases covers what the corpus does not, and seeded positions.
func TestCases(t *testing.T) { allCases(t, same) }

// allCases runs check on every case, from the start and seeded.
func allCases(t *testing.T, check func(t *testing.T, name, src string, line, column int) bool) {
	for name, src := range cases() {
		check(t, name, src, 0, 0)
		check(t, name+", seeded", src, 7, 12)
	}
}

func benchmark(b *testing.B, lex func(string) antlr.Lexer) {
	fs := files(b)
	b.SetBytes(int64(corpus.Bytes(fs)))
	b.ReportAllocs()
	for b.Loop() {
		for _, f := range fs {
			l := lex(f.Src)
			for l.NextToken().GetTokenType() != antlr.TokenEOF {
			}
		}
	}
}

// BenchmarkANTLR lexes the corpus with ANTLR's lexer, warm.
func BenchmarkANTLR(b *testing.B) {
	benchmark(b, func(src string) antlr.Lexer {
		l := antlrgala.NewgalaLexer(antlr.NewInputStream(src))
		l.RemoveErrorListeners()
		return l
	})
}

// BenchmarkGalop lexes the corpus with galop's lexer.
func BenchmarkGalop(b *testing.B) {
	benchmark(b, func(src string) antlr.Lexer {
		l := antlrgala.NewgalaGalopLexer(antlr.NewInputStream(src))
		l.RemoveErrorListeners()
		return l
	})
}

// tokenList renders every token a lexer gives, all channels, through EOF, and
// its errors: what the native lexer must reproduce. Positions are the line and
// column; the native token's byte offsets are checked through its text.
func tokenList(src string, line, column int) lexed {
	var r lexed
	l := antlrgala.NewgalaLexer(antlr.NewInputStream(src))
	l.RemoveErrorListeners()
	l.AddErrorListener(recorder{errors: &r.errors})
	if line > 0 {
		sim := l.Interpreter.(*antlr.LexerATNSimulator)
		sim.Line, sim.CharPositionInLine = line, column
	}
	for {
		t := l.NextToken()
		r.tokens = append(r.tokens, fmt.Sprintf("type=%d text=%q channel=%d at %d:%d", t.GetTokenType(), t.GetText(), t.GetChannel(), t.GetLine(), t.GetColumn()))
		if t.GetTokenType() == antlr.TokenEOF {
			return r
		}
	}
}

// nativeList renders the native lexer's tokens the way tokenList renders
// ANTLR's, merging the hidden tokens back into place.
func nativeList(src string, line, column int) lexed {
	var r lexed
	if line == 0 {
		line, column = 1, 0
	}
	ts := nativegala.GalaLexAt(src, line, column)
	for _, e := range ts.Errors {
		r.errors = append(r.errors, fmt.Sprintf("%d:%d %s", e.Line, e.Column, e.Msg))
	}
	render := func(t nativegala.GalaToken, channel int32) string {
		return fmt.Sprintf("type=%d text=%q channel=%d at %d:%d", t.Type, t.Text(src), channel, t.Line, t.Column)
	}
	h := 0
	for i, t := range ts.Tokens {
		for h < len(ts.Hidden) && int(ts.Hidden[h].Next) == i {
			r.tokens = append(r.tokens, render(ts.Hidden[h].GalaToken, ts.Hidden[h].Channel))
			h++
		}
		r.tokens = append(r.tokens, render(t, nativegala.GalaDefaultChannel))
	}
	return r
}

// sameNative compares the native lexer with ANTLR's on src.
func sameNative(t *testing.T, name, src string, line, column int) bool {
	t.Helper()
	return compare(t, name, tokenList(src, line, column), "native", nativeList(src, line, column))
}

// TestNativeCorpus lexes every corpus file and its variants with ANTLR's lexer
// and the native flavor's.
func TestNativeCorpus(t *testing.T) { corpusAndVariants(t, sameNative) }

// TestNativeCases is TestCases for the native lexer.
func TestNativeCases(t *testing.T) { allCases(t, sameNative) }

// BenchmarkNative lexes the corpus with the native lexer into token lists.
func BenchmarkNative(b *testing.B) {
	fs := files(b)
	b.SetBytes(int64(corpus.Bytes(fs)))
	b.ReportAllocs()
	for b.Loop() {
		for _, f := range fs {
			nativegala.GalaLex(f.Src)
		}
	}
}
