// Package errors checks galop's syntax errors against ANTLR's on Errors.g4, a
// grammar whose decisions galop settles without runtime prediction.
package errors

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	ea "github.com/martianoff/galop/differential/errors/antlr"
	en "github.com/martianoff/galop/differential/errors/native"
)

// programs are valid inputs; the test mutates them.
var programs = []string{
	`let x = 1 + f(2, "a") * -y;
fn main(a: int, b: list[int]) {
	// a comment
	if a < b { return a; } else if b { x = [ 1 2 ]; } else return;
	loop f(); { ; } end
	# x ;
	pair a + 1 - b + ;
	each x (y) do { }
	skip a, b, until c;
}
type point { x int; y *int; }
type alias = list[*point]
// the end`,
	`fn f() { return !true == false; }
let type = (type);
fn h() { each (a) b do { pair 1 + ; } skip until x; }`,
	`type t { a b; }
fn g(x: t) { loop ; end ; g(x); }`,
}

// insertions are text the test puts between tokens.
var insertions = []string{
	"let", "fn", "type", "=", ";", "(", ")", "{", "}", "[", "]", ",", ":", "*", "-", "!", "#",
	"return", "if", "else", "loop", "end", "pair", "each", "do", "skip", "until",
	"x", "1", `"s"`, "true", "==", "// c\n",
}

func TestSyntaxErrorsMatchANTLR(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	checked := 0
	for i := 0; i < 3000; i++ {
		src := mutate(r, programs[r.Intn(len(programs))])
		want := antlrFirstError(src)
		_, nerr := en.ParseProg(en.ErrorsLex(src))
		stream := antlr.NewCommonTokenStream(ea.NewErrorsGalopLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel)
		_, aerr := ea.NewErrorsGalopParser(stream).Prog()
		for _, got := range []struct {
			flavor string
			err    error
		}{{"native", nerr}, {"antlr", aerr}} {
			if text(got.err) != want {
				t.Errorf("mutant %d, %s flavor:\n galop %s\n ANTLR %s\n%s", i, got.flavor, text(got.err), want, src)
			}
		}
		if want != "" {
			checked++
		}
	}
	if checked < 1500 {
		t.Errorf("only %d mutants have a syntax error, want at least 1500", checked)
	}
}

func text(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// mutate drops, doubles or inserts text at a random token of src.
func mutate(r *rand.Rand, src string) string {
	lexer := ea.NewErrorsLexer(antlr.NewInputStream(src))
	var toks []antlr.Token
	for t := lexer.NextToken(); t.GetTokenType() != antlr.TokenEOF; t = lexer.NextToken() {
		toks = append(toks, t)
	}
	tok := toks[r.Intn(len(toks))]
	start, stop := tok.GetStart(), tok.GetStop()+1
	switch r.Intn(3) {
	case 0:
		return src[:start] + src[stop:]
	case 1:
		return src[:stop] + " " + src[start:]
	default:
		return src[:start] + insertions[r.Intn(len(insertions))] + " " + src[start:]
	}
}

type firstError struct {
	antlr.DefaultErrorListener
	msg string
}

func (e *firstError) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	if e.msg == "" {
		e.msg = fmt.Sprintf("line %d:%d %s", line, column, msg)
	}
}

// antlrFirstError is the first syntax error ANTLR reports for src as prog.
func antlrFirstError(src string) string {
	return antlrFirstErrorAs(src, func(p *ea.ErrorsParser) { p.Prog() })
}

// antlrFirstErrorAs is the first syntax error ANTLR reports for src parsed by
// entry, with SLL prediction and then, after an error, with LL prediction
// from the start, as the GALA compiler does. Lexical errors count; it is empty
// when there is no error.
func antlrFirstErrorAs(src string, entry func(*ea.ErrorsParser)) string {
	var msg string
	for _, mode := range []int{antlr.PredictionModeSLL, antlr.PredictionModeLL} {
		lexer := ea.NewErrorsLexer(antlr.NewInputStream(src))
		lexer.RemoveErrorListeners()
		lexErrs := &firstError{}
		lexer.AddErrorListener(lexErrs)
		p := ea.NewErrorsParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
		p.RemoveErrorListeners()
		errs := &firstError{}
		p.AddErrorListener(errs)
		p.GetInterpreter().SetPredictionMode(mode)
		entry(p)
		if lexErrs.msg != "" {
			return lexErrs.msg
		}
		if msg = errs.msg; msg == "" {
			return ""
		}
	}
	return strings.TrimSpace(msg)
}

// TestEntryRuleWithoutEOF checks errors in snippet, an entry rule no rule
// calls and that does not end with EOF: past its end ANTLR expects EOF.
// (Input ANTLR accepts up to some token and then stops, like `eval x + y`, is
// left out: galop's parse of an entry rule without EOF rejects the rest.)
func TestEntryRuleWithoutEOF(t *testing.T) {
	for _, src := range []string{"eval x x", "eval x", "eval", "eval +", "eval x + 1", "eval x ; 1", "x", "eval x - \"s\""} {
		want := antlrFirstErrorAs(src, func(p *ea.ErrorsParser) { p.Snippet() })
		_, nerr := en.ParseSnippet(en.ErrorsLex(src))
		stream := antlr.NewCommonTokenStream(ea.NewErrorsGalopLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel)
		_, aerr := ea.NewErrorsGalopParser(stream).Snippet()
		for _, got := range []struct {
			flavor string
			err    error
		}{{"native", nerr}, {"antlr", aerr}} {
			if text(got.err) != want {
				t.Errorf("%q, %s flavor:\n galop %s\n ANTLR %s", src, got.flavor, text(got.err), want)
			}
		}
	}
}

// TestTokenTypeOutsideTheVocabulary feeds the native parser token types no
// rule matches, which a program may set before parsing: the error replay
// reports a syntax error at that token instead of failing.
func TestTokenTypeOutsideTheVocabulary(t *testing.T) {
	for _, typ := range []int32{99, -5, 0} {
		ts := en.ErrorsLex("let x = 1;")
		ts.Tokens[2].Type = typ
		_, err := en.ParseProg(ts)
		if err == nil || !strings.HasPrefix(err.Error(), "line 1:6 ") {
			t.Errorf("token type %d: got %v, want a syntax error at 1:6", typ, err)
		}
	}
}
