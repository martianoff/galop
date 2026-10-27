// Package nativegala holds galop's parser for gala.g4 in the Go target's
// native flavor, generated at build time, and GALA's line-break rule ported
// to its token list. It does not depend on ANTLR.
package nativegala

import "strings"

// bom is the UTF-8 encoding of U+FEFF, the byte order mark.
const bom = "\xef\xbb\xbf"

// Tokens is the token list the GALA compiler parses src from: galop's native
// lexer with GALA's line-break re-typing applied, byte order mark stripped.
func Tokens(src string) *GalaTokens {
	ts := GalaLex(strings.TrimPrefix(src, bom))
	RetypeLines(ts)
	return ts
}

// Parse parses src as a GALA source file. Lexical errors count as errors, as
// in the GALA compiler.
func Parse(src string) (SourceFileNode, error) {
	ts := Tokens(src)
	if len(ts.Errors) > 0 {
		return SourceFileNode{}, &ts.Errors[0]
	}
	return ParseSourceFile(ts)
}

// ParseByRerun parses src as Parse does, but after a syntax error of the SLL
// pass it reruns the whole parse in LL mode, as ANTLR does, instead of
// repairing the failed parse: the reference the repair is checked against.
func ParseByRerun(src string) (SourceFileNode, error) {
	ts := Tokens(src)
	if len(ts.Errors) > 0 {
		return SourceFileNode{}, &ts.Errors[0]
	}
	t, root, err := newgalaGalopParser(ts).run((*galaGalopParser).ruleSourceFile, galaGalopRuleStartState[GalaRULE_sourceFile], false)
	if err != nil {
		return SourceFileNode{}, err
	}
	return SourceFileNode{GalaNodeRef{t, root}}, nil
}
