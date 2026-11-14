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

// openers start a token that runs to the end of its line or of the input when
// nothing closes it: an unterminated string, character or raw string.
var openers = []string{"\"", "'", "`"}

// invalidUTF8 are bytes that are not valid UTF-8, which ANTLR reads as U+FFFD
// each: an invalid byte, a lone continuation byte, a sequence cut short.
var invalidUTF8 = []string{"\x94", "\xff", "\x80", "\xe2\x82"}

// withStrayChars breaks src lexically one to three times, and half the time
// also mutates it (see mutate.Mutate), so that lexical and syntax errors mix.
// Each break puts text where a token starts or ends, inside a token, at the
// end of a line or at the end of the input: one of strayChars, a run of them,
// an opener left unclosed, or bytes that are not valid UTF-8.
func withStrayChars(r *rand.Rand, src string) string {
	if r.Intn(2) == 0 {
		src = mutate.Mutate(r, src)
	}
	for n := 1 + r.Intn(3); n > 0; n-- {
		toks := nativegala.Tokens(src).Tokens
		tok := toks[r.Intn(len(toks))]
		var at int
		switch r.Intn(5) {
		case 0:
			at = int(tok.Start)
		case 1:
			at = int(tok.End)
		case 2:
			// Inside the token, on a rune boundary (it splits it in two).
			at = int(tok.Start)
			if text := src[tok.Start:tok.End]; len(text) > 1 {
				for i := range text {
					if i > 0 && r.Intn(2) == 0 {
						at = int(tok.Start) + i
						break
					}
				}
			}
		case 3:
			// At the end of the token's line.
			at = int(tok.End)
			if nl := strings.IndexByte(src[at:], '\n'); nl >= 0 {
				at += nl
			} else {
				at = len(src)
			}
		default:
			at = len(src)
		}
		var text string
		switch r.Intn(5) {
		case 0:
			text = openers[r.Intn(len(openers))]
		case 1:
			text = strings.Repeat(strayChars[r.Intn(len(strayChars))], 2+r.Intn(3))
		case 2:
			text = invalidUTF8[r.Intn(len(invalidUTF8))]
		default:
			text = strayChars[r.Intn(len(strayChars))]
		}
		src = src[:at] + text + src[at:]
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
