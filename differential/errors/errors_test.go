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
fn g(x: t) { loop ; end ; g(x)(1); }`,
}

// insertions are text the test puts between tokens.
var insertions = []string{
	"let", "fn", "type", "=", ";", "(", ")", "{", "}", "[", "]", ",", ":", "*", "-", "!", "#",
	"return", "if", "else", "loop", "end", "x", "1", `"s"`, "true", "==", "// c\n",
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

// antlrFirstError is the first syntax error ANTLR reports for src, parsed with
// SLL prediction and then, after an error, with LL prediction from the start,
// as the GALA compiler does. Lexical errors count; it is empty when there is
// no error.
func antlrFirstError(src string) string {
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
		p.Prog()
		if lexErrs.msg != "" {
			return lexErrs.msg
		}
		if msg = errs.msg; msg == "" {
			return ""
		}
	}
	return strings.TrimSpace(msg)
}
