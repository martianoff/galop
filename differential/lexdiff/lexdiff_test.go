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

// same compares the two lexers on src and reports the first difference.
func same(t *testing.T, name, src string, line, column int) bool {
	t.Helper()
	a, g := antlrLex(src, line, column), galopLex(src, line, column)
	if strings.Join(a.errors, "\n") != strings.Join(g.errors, "\n") {
		t.Errorf("%s: errors differ\nANTLR: %q\ngalop: %q", name, a.errors, g.errors)
		return false
	}
	for i := 0; i < len(a.tokens) && i < len(g.tokens); i++ {
		if a.tokens[i] != g.tokens[i] {
			t.Errorf("%s: token %d differs\nANTLR: %s\ngalop: %s", name, i, a.tokens[i], g.tokens[i])
			return false
		}
	}
	if len(a.tokens) != len(g.tokens) {
		t.Errorf("%s: %d tokens from ANTLR, %d from galop", name, len(a.tokens), len(g.tokens))
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
func TestCorpus(t *testing.T) {
	fs := files(t)
	identical, variantsIdentical, variantsTotal := 0, 0, 0
	for _, f := range fs {
		if same(t, f.Name, f.Src, 0, 0) {
			identical++
		}
		for name, src := range variants(f.Src) {
			variantsTotal++
			if same(t, f.Name+" ("+name+")", src, 0, 0) {
				variantsIdentical++
			}
		}
	}
	t.Logf("%d/%d files identical; %d/%d variants identical", identical, len(fs), variantsIdentical, variantsTotal)
}

// TestCases covers what the corpus does not: empty input, interpolations
// nested past the static DFA's rule-call depth, and seeded positions.
func TestCases(t *testing.T) {
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
	for name, src := range cases {
		same(t, name, src, 0, 0)
		same(t, name+", seeded", src, 7, 12)
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
