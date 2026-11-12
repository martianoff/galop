package diff

import (
	"flag"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/mutate"
	"github.com/martianoff/galop/differential/nativegala"
)

var lexErrorInputs = flag.Int("lex_error_inputs", 300, "TestLexAndSyntaxErrorOrder: how many inputs with lexical errors to check, per kind")

// strayChars are characters GALA's lexer has no token for.
var strayChars = []string{"#", "@", "$", "`", "\\", "\x01", "¤"}

// withStrayChars puts one to three of strayChars at token boundaries of src,
// and half the time also mutates it (see mutate.Mutate), so that lexical and
// syntax errors mix.
func withStrayChars(r *rand.Rand, src string) string {
	if r.Intn(2) == 0 {
		src = mutate.Mutate(r, src)
	}
	for n := 1 + r.Intn(3); n > 0; n-- {
		toks := nativegala.Tokens(src).Tokens
		at := int(toks[r.Intn(len(toks))].Start)
		src = src[:at] + strayChars[r.Intn(len(strayChars))] + src[at:]
	}
	return src
}

// interleaved merges ts's token recognition errors with a recovering parse's
// syntax errors in the order ANTLR reports them: a lexical error comes before
// the first syntax error reported after its token was read (ErrorAt <= Read),
// and one past the parse's last read token (Tree.Read) is never reported.
func interleaved(ts *nativegala.GalaTokens, errs []nativegala.GalaParseError, read int) []string {
	var out []string
	lex := 0
	emitLex := func(upTo int) {
		for ; lex < len(ts.Errors) && int(ts.ErrorAt[lex]) <= upTo; lex++ {
			out = append(out, renderError(ts.Errors[lex]))
		}
	}
	for _, e := range errs {
		emitLex(e.Read)
		out = append(out, renderError(e.GalaSyntaxError))
	}
	emitLex(read)
	return out
}

// TestLexAndSyntaxErrorOrder checks that a recovering parse's Read places
// token recognition errors among syntax errors as ANTLR reports them, for
// whole files and for expressions re-parsed at their position, on corpus
// files and expressions with stray characters (and often a mutation too).
func TestLexAndSyntaxErrorOrder(t *testing.T) {
	srcs := mutationSources(t)
	exprs := corpusExpressions(t)
	for _, kind := range []struct {
		name string
		one  func(r *rand.Rand) (src string, want, got []string)
	}{
		{"files", func(r *rand.Rand) (string, []string, []string) {
			src := withStrayChars(r, srcs[r.Intn(len(srcs))])
			ts := nativegala.Tokens(src)
			if len(ts.Errors) == 0 {
				return "", nil, nil
			}
			root, errs := nativegala.ParseSourceFileWithRecovery(ts)
			return src, gala.AllErrors(src, false, 0, 0), interleaved(ts, errs, root.Tree().Read)
		}},
		{"expressions", func(r *rand.Rand) (string, []string, []string) {
			x := exprs[r.Intn(len(exprs))]
			src := withStrayChars(r, x.text)
			ts := nativegala.TokensAt(src, x.line, x.column)
			if len(ts.Errors) == 0 {
				return "", nil, nil
			}
			root, errs := nativegala.GalaParseWithRecovery(ts, nativegala.GalaRULE_expression)
			return src, gala.AllErrors(src, true, x.line, x.column), interleaved(ts, errs, root.Tree().Read)
		}},
	} {
		r := rand.New(rand.NewSource(*mutantSeed))
		checked, same, mixed := 0, 0, 0
		for tries := 0; checked < *lexErrorInputs && tries < 50**lexErrorInputs; tries++ {
			src, want, got := kind.one(r)
			if want == nil && got == nil {
				continue
			}
			checked++
			if slices.ContainsFunc(want, func(s string) bool { return !strings.Contains(s, "token recognition error") }) {
				mixed++
			}
			if slices.Equal(want, got) {
				same++
				continue
			}
			if checked-same <= 10 {
				t.Errorf("%s %d:\n ANTLR: %s\n galop: %s\n%q", kind.name, checked,
					strings.Join(want, "\n        "), strings.Join(got, "\n        "), src)
			}
		}
		if checked == 0 {
			t.Fatalf("%s: no input with a lexical error", kind.name)
		}
		t.Logf("%s: %d inputs with lexical errors (%d with syntax errors too), %d in ANTLR's order", kind.name, checked, mixed, same)
	}
}

// corpusExpression is an expression cut from a corpus file, with the line and
// column it starts at.
type corpusExpression struct {
	file, text   string
	line, column int
}

// corpusExpressions are every fifth expression of every fourth corpus file
// that parses.
func corpusExpressions(tb testing.TB) []corpusExpression {
	var out []corpusExpression
	for fi, f := range files(tb) {
		if fi%4 != 0 {
			continue
		}
		root, err := nativegala.Parse(f.Src)
		if err != nil {
			continue
		}
		src := gala.StripBOM(f.Src)
		k := 0
		root.Walk(func(n nativegala.GalaNodeRef) bool {
			if n.Rule() != nativegala.GalaRULE_expression || n.StopIndex() < n.StartIndex() {
				return true
			}
			if k++; k%5 == 0 {
				out = append(out, corpusExpression{f.Name, src[n.Start().Start:n.Stop().End], int(n.Start().Line), int(n.Start().Column)})
			}
			return true
		})
	}
	return out
}
