// Package milvus measures galop on Milvus's filter expressions: the grammar is
// Milvus's Plan.g4 (testdata/grammars), whose expression rule is
// left-recursive, and the input is exprs.json, the expressions of Milvus's plan
// parser tests (see the differential README for its source).
//
// It parses them three ways: galop's native flavor, ANTLR's default LL parse, which galop reproduces, and
// Milvus's own driver around ANTLR's parser.
package milvus

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/antlr4-go/antlr/v4"

	plan "github.com/martianoff/galop/differential/convert/plan/antlr"
	"github.com/martianoff/galop/differential/convert/plan/native"
)

// Load reads the expressions from a JSON array of strings.
func Load(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var exprs []string
	if err := json.Unmarshal(data, &exprs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return exprs, nil
}

// Native parses q with galop's native flavor and reports whether it parsed.
func Native(q string) bool {
	_, err := native.ParseExpr(native.PlanLex(q))
	return err == nil
}

// Lex runs galop's native lexer alone.
func Lex(q string) { native.PlanLex(q) }

// errorCount counts syntax errors.
type errorCount struct {
	*antlr.DefaultErrorListener
	n int
}

func (c *errorCount) SyntaxError(antlr.Recognizer, any, int, int, string, antlr.RecognitionException) {
	c.n++
}

// LL parses q with a fresh ANTLR parser in its default LL mode, as a
// generated parser is used out of the box, and reports whether it parsed.
func LL(q string) bool {
	errs := &errorCount{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	lexer := plan.NewPlanLexer(antlr.NewInputStream(q))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	p := plan.NewPlanParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	p.Expr()
	return errs.n == 0
}

var (
	lexers  = sync.Pool{New: func() any { return plan.NewPlanLexer(nil) }}
	parsers = sync.Pool{New: func() any { return plan.NewPlanParser(nil) }}
)

// Driver parses q as Milvus's plan parser does (handleInternal in
// internal/parser/planparserv2, without its cache of parsed plans): a pooled
// lexer and parser, SLL prediction first, and LL again only after an SLL
// syntax error. It reports whether q parsed.
func Driver(q string) bool {
	lexer := lexers.Get().(*plan.PlanLexer)
	defer lexers.Put(lexer)
	errs := &errorCount{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	lexer.SetInputStream(antlr.NewInputStream(q))
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parsers.Get().(*plan.PlanParser)
	defer parsers.Put(p)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	p.BuildParseTrees = true
	p.SetInputStream(tokens)
	p.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	interp := p.GetInterpreter()
	interp.SetPredictionMode(antlr.PredictionModeSLL)
	p.Expr()
	if errs.n > 0 {
		errs.n = 0
		tokens.Seek(0)
		p.SetInputStream(tokens)
		interp.SetPredictionMode(antlr.PredictionModeLL)
		p.Expr()
	}
	lexer.SetInputStream(nil)
	p.SetInputStream(nil)
	return errs.n == 0
}
