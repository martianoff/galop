package diff

import (
	"fmt"
	"slices"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
)

// atResult is what one parser makes of an expression re-parsed at a position:
// its tree, its tokens with their lines and columns, the type of the token it
// stopped before, and its error.
type atResult struct {
	tree   string
	tokens []string
	next   int
	err    string
}

// errText is err's text, or "".
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// streamTokens renders a stream's default-channel tokens.
func streamTokens(stream *antlr.CommonTokenStream) []string {
	stream.Fill()
	var out []string
	for _, t := range stream.GetAllTokens() {
		if t.GetChannel() == antlr.TokenDefaultChannel {
			out = append(out, fmt.Sprintf("%d %q %d:%d", t.GetTokenType(), t.GetText(), t.GetLine(), t.GetColumn()))
		}
	}
	return out
}

// antlrAt re-parses text as one expression at line and column the way the
// GALA compiler does with ANTLR.
func antlrAt(text string, line, column int) atResult {
	stream := gala.TokensAt(text, line, column)
	p := antlrgala.NewgalaParser(stream)
	errs := &firstError{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	tree := gala.TreeString(p.Expression())
	return atResult{tree, streamTokens(stream), stream.LT(1).GetTokenType(), errs.text}
}

// galopAt is antlrAt with galop's antlr flavor.
func galopAt(text string, line, column int) atResult {
	stream := gala.GalopTokensAt(text, line, column)
	ctx, err := antlrgala.NewgalaGalopParser(stream).Expression()
	if err != nil {
		return atResult{err: err.Error()}
	}
	return atResult{gala.TreeString(ctx), streamTokens(stream), stream.LT(1).GetTokenType(), ""}
}

// nativeAt is antlrAt with galop's native flavor.
func nativeAt(text string, line, column int) atResult {
	ts := nativegala.TokensAt(text, line, column)
	root, err := nativegala.ParseExpression(ts)
	if err != nil {
		return atResult{err: err.Error()}
	}
	var toks []string
	for _, t := range ts.Tokens {
		toks = append(toks, fmt.Sprintf("%d %q %d:%d", t.Type, t.Text(text), t.Line, t.Column))
	}
	t := root.Tree()
	return atResult{root.ToStringTree(), toks, int(t.Tokens[t.Next].Type), ""}
}

// firstError keeps the text of ANTLR's first syntax error.
type firstError struct {
	*antlr.DefaultErrorListener
	text string
}

func (e *firstError) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	if e.text == "" {
		e.text = fmt.Sprintf("%d:%d: %s", line, column, msg)
	}
}

// TestParseAtMatchesANTLR re-parses expressions cut from the corpus at their
// line and column, as the GALA compiler re-parses interpolated expressions and
// default values: galop's antlr flavor and native flavor must give ANTLR's
// tree, tokens with ANTLR's lines and columns, and stop before the token ANTLR
// stops before. Each expression is also parsed with a stray token after it,
// which the rule leaves for the caller to report.
func TestParseAtMatchesANTLR(t *testing.T) {
	checked := 0
	for fi, f := range files(t) {
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
			if k++; k%5 != 0 {
				return true
			}
			text := src[n.Start().Start:n.Stop().End]
			line, column := int(n.Start().Line), int(n.Start().Column)
			for _, in := range []string{text, text + " )"} {
				checked++
				want := antlrAt(in, line, column)
				for _, p := range []struct {
					name string
					got  atResult
				}{{"antlr flavor", galopAt(in, line, column)}, {"native", nativeAt(in, line, column)}} {
					switch g := p.got; {
					case want.err != "" || g.err != "":
						if (want.err == "") != (g.err == "") {
							t.Errorf("%s at %d:%d %q: %s error %q, ANTLR %q", f.Name, line, column, in, p.name, g.err, want.err)
						}
					case g.tree != want.tree:
						t.Errorf("%s at %d:%d %q: %s tree differs\n%s", f.Name, line, column, in, p.name, firstDifference(want.tree, g.tree))
					case !slices.Equal(g.tokens, want.tokens):
						t.Errorf("%s at %d:%d %q: %s tokens\n%v\nwant\n%v", f.Name, line, column, in, p.name, g.tokens, want.tokens)
					case g.next != want.next:
						t.Errorf("%s at %d:%d %q: %s stops before token type %d, ANTLR %d", f.Name, line, column, in, p.name, g.next, want.next)
					}
				}
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no expression was re-parsed")
	}
	t.Logf("%d expressions re-parsed at their positions", checked)
}
