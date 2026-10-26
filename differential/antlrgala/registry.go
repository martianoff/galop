// Package antlrgala holds ANTLR 4.13.1's Go parser for gala.g4, generated at
// build time, and any parser that builds the same tree types.
package antlrgala

import "github.com/antlr4-go/antlr/v4"

// SameTypes holds the parsers that live in this package next to ANTLR's
// generated code and build its context types, keyed by the name the harness
// tools select them by. galop's antlr flavor joins the harness by adding its
// file to this package (and to the go_library's srcs) with an init that
// registers it:
//
//	func init() { SameTypes["galop"] = ParseGalop }
//
// A parser reads the whole stream and returns the sourceFile tree, plus an
// error when the input has syntax errors.
var SameTypes = map[string]func(stream antlr.TokenStream) (antlr.ParserRuleContext, error){}
