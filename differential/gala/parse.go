// Package gala runs ANTLR's parser for gala.g4 the way the GALA compiler does:
// the same token stream (with GALA's line-break re-typing, see newline.go),
// the same prediction modes, and per-parse prediction-context caches so that
// files can be parsed on several goroutines at once.
package gala

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
)

// The prediction modes ParseANTLR accepts.
const (
	// SLL is ANTLR's cheaper prediction mode, which ignores the call stack.
	SLL = antlr.PredictionModeSLL
	// LL is ANTLR's full-context prediction mode.
	LL = antlr.PredictionModeLL
)

// Parser parses a whole token stream of a GALA file into its sourceFile tree.
// The error reports syntax errors. ANTLR's parser then still returns its
// error-recovered tree; galop's generated parser, which has no error recovery
// yet, returns no tree. Both fit this type.
type Parser func(stream antlr.TokenStream) (antlr.ParserRuleContext, error)

// bom is the UTF-8 encoding of U+FEFF, the byte order mark.
const bom = "\xef\xbb\xbf"

// StripBOM drops a leading UTF-8 byte order mark, as the GALA compiler does
// before it lexes a file: the lexer has no rule for U+FEFF.
func StripBOM(src string) string { return strings.TrimPrefix(src, bom) }

// Tokens is the token stream the GALA compiler parses src from: ANTLR's lexer
// with GALA's line-break re-typing applied, BOM stripped. Lexical errors are
// dropped; ParseANTLR reports them.
func Tokens(src string) *antlr.CommonTokenStream {
	return tokens(src, nil)
}

// GalopTokens is Tokens with galop's generated lexer in place of ANTLR's.
func GalopTokens(src string) *antlr.CommonTokenStream {
	lexer := antlrgala.NewgalaGalopLexer(antlr.NewInputStream(StripBOM(src)))
	lexer.RemoveErrorListeners()
	return newTokenStream(lexer)
}

// TokensAt is Tokens for text whose first character sits at line and
// column of an enclosing file (line from 1, column from 0): the lexer counts
// from there, as the GALA compiler seeds it to re-parse an interpolated
// expression or a default value.
func TokensAt(src string, line, column int) *antlr.CommonTokenStream {
	lexer := antlrgala.NewgalaLexer(antlr.NewInputStream(src))
	isolateLexerCaches(lexer.BaseLexer)
	lexer.RemoveErrorListeners()
	seed(lexer.BaseLexer, line, column)
	return newTokenStream(lexer)
}

// GalopTokensAt is TokensAt with galop's generated lexer in place of ANTLR's.
func GalopTokensAt(src string, line, column int) *antlr.CommonTokenStream {
	lexer := antlrgala.NewgalaGalopLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	seed(lexer.BaseLexer, line, column)
	return newTokenStream(lexer)
}

// seed makes a lexer count lines and columns from line and column.
func seed(lexer *antlr.BaseLexer, line, column int) {
	sim := lexer.Interpreter.(*antlr.LexerATNSimulator)
	sim.Line, sim.CharPositionInLine = line, column
}

func tokens(src string, errs antlr.ErrorListener) *antlr.CommonTokenStream {
	lexer := antlrgala.NewgalaLexer(antlr.NewInputStream(StripBOM(src)))
	isolateLexerCaches(lexer.BaseLexer)
	lexer.RemoveErrorListeners()
	if errs != nil {
		lexer.AddErrorListener(errs)
	}
	return newTokenStream(lexer)
}

// ParseANTLR parses src with ANTLR under the given prediction mode (SLL or
// LL) and returns the sourceFile tree with every lexical and syntax error.
func ParseANTLR(src string, mode int) (antlr.ParserRuleContext, []error) {
	errs := &errorListener{}
	tree := parse(tokens(src, errs), mode, errs, nil)
	return tree, errs.errors
}

// parse parses stream with ANTLR, predicting over the DFA set dfa, or over the
// shared one (see sharedDFA) if dfa is nil.
func parse(stream antlr.TokenStream, mode int, errs *errorListener, dfa []*antlr.DFA) antlr.ParserRuleContext {
	p := antlrgala.NewgalaParser(stream)
	isolateParserCaches(p.BaseParser, dfa)
	p.GetInterpreter().SetPredictionMode(mode)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	return p.SourceFile()
}

// ANTLRParser is ANTLR's parser under one prediction mode.
func ANTLRParser(mode int) Parser {
	return func(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
		errs := &errorListener{}
		tree := parse(stream, mode, errs, nil)
		return tree, errors.Join(errs.errors...)
	}
}

// Lenient is the GALA compiler's strategy: parse with SLL prediction and, if
// that reports a syntax error, parse again from the start with LL. The tree
// and errors are those of the attempt that is kept.
func Lenient(stream antlr.TokenStream) (antlr.ParserRuleContext, error) {
	tree, err := ANTLRParser(SLL)(stream)
	if err == nil {
		return tree, nil
	}
	stream.Seek(0)
	return ANTLRParser(LL)(stream)
}

// Parsers returns every parser the harness tools can select, by name: ANTLR
// under each strategy, plus the parsers registered in antlrgala.SameTypes.
func Parsers() map[string]Parser {
	ps := map[string]Parser{
		"antlr-sll":     ANTLRParser(SLL),
		"antlr-ll":      ANTLRParser(LL),
		"antlr-lenient": Lenient,
	}
	for name, p := range antlrgala.SameTypes {
		ps[name] = p
	}
	return ps
}

// ParserNames lists the names Parsers accepts, for flag help.
func ParserNames() string {
	return strings.Join(slices.Sorted(maps.Keys(Parsers())), ", ")
}

// RuleNames are gala.g4's parser rule names, indexed by rule index.
func RuleNames() []string {
	antlrgala.GalaParserInit()
	return antlrgala.GalaParserStaticData.RuleNames
}

// TreeString is ANTLR's ToStringTree rendering of tree, with rule names.
func TreeString(tree antlr.Tree) string {
	return antlr.TreesStringTree(tree, RuleNames(), nil)
}

// errorListener collects lexical and syntax errors as "line:col: message",
// naming tokens the way the GALA compiler does (see hideNewlineTokens).
type errorListener struct {
	*antlr.DefaultErrorListener
	errors []error
}

func (l *errorListener) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	l.errors = append(l.errors, fmt.Errorf("%d:%d: %s", line, column, hideNewlineTokens(msg)))
}

// sharedDFA builds one DFA set per ATN, once, and shares it between parses.
// Concurrent parses may extend these DFAs; the ANTLR runtime serialises every
// such change with the ATN's own locks, so sharing keeps ANTLR's cross-parse
// DFA caching without a race. The generated static DFA set is unexported,
// hence this copy.
type sharedDFA struct {
	once sync.Once
	dfa  []*antlr.DFA
}

func (s *sharedDFA) get(atn *antlr.ATN) []*antlr.DFA {
	s.once.Do(func() { s.dfa = newDFA(atn) })
	return s.dfa
}

// newDFA is an empty DFA set for atn.
func newDFA(atn *antlr.ATN) []*antlr.DFA {
	dfa := make([]*antlr.DFA, len(atn.DecisionToState))
	for i, ds := range atn.DecisionToState {
		dfa[i] = antlr.NewDFA(ds, i)
	}
	return dfa
}

var lexerDFA, parserDFA sharedDFA

// isolateLexerCaches gives a lexer its own PredictionContextCache, the one
// piece of shared simulator state the ANTLR runtime does not lock, and keeps
// the shared DFA set. The GALA compiler does the same for every parse.
func isolateLexerCaches(l *antlr.BaseLexer) {
	atn := l.GetATN()
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, lexerDFA.get(atn), antlr.NewPredictionContextCache())
}

// isolateParserCaches is the parser-side counterpart of isolateLexerCaches,
// with the DFA set dfa in place of the shared one unless dfa is nil.
func isolateParserCaches(p *antlr.BaseParser, dfa []*antlr.DFA) {
	atn := p.GetATN()
	if dfa == nil {
		dfa = parserDFA.get(atn)
	}
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, dfa, antlr.NewPredictionContextCache())
}

// FirstSyntaxError is the first syntax error the GALA compiler's strategy
// (see Lenient) reports for src, as "line:col: message", and whether there
// is one. Lexical errors are left out: ParseANTLR reports them.
//
// Both attempts share a DFA set of their own, which starts empty. ANTLR's
// error can depend on the DFA states it already holds (a failed prediction
// takes a kept state's record of which configurations left the decision's
// rule), so over the shared set the error would depend on what the process
// parsed before.
func FirstSyntaxError(src string) (string, bool) {
	stream := Tokens(src)
	dfa := newDFA(antlrgala.NewgalaParser(stream).GetATN())
	errs := &errorListener{}
	if parse(stream, SLL, errs, dfa); len(errs.errors) == 0 {
		return "", false
	}
	stream.Seek(0)
	errs = &errorListener{}
	if parse(stream, LL, errs, dfa); len(errs.errors) == 0 {
		return "", false
	}
	return errs.errors[0].Error(), true
}

// HideNewlineTokens is hideNewlineTokens for messages from other parsers, so
// that their errors read as the GALA compiler's do.
func HideNewlineTokens(msg string) string { return hideNewlineTokens(msg) }
