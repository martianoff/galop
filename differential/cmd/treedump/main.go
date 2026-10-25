// Command treedump prints ANTLR's ToStringTree rendering of each input file's
// parse tree, one file per block:
//
//	== std/option.gala
//	(sourceFile (packageClause package std) ...)
//
// Syntax errors go to stderr and make the exit status 1.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
)

func main() {
	var src corpus.Source
	src.RegisterFlags(flag.CommandLine)
	parserName := flag.String("parser", "antlr-lenient", "parser to use: "+gala.ParserNames()+
		" (antlr-lenient is the GALA compiler's SLL, then LL on a syntax error)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: treedump [-parser name] [-dir dir | -manifest file | file...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	src.Files = flag.Args()

	parse, ok := gala.Parsers()[*parserName]
	if !ok {
		fmt.Fprintf(os.Stderr, "treedump: unknown parser %q (have %s)\n", *parserName, gala.ParserNames())
		os.Exit(2)
	}
	files, err := src.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "treedump:", err)
		os.Exit(2)
	}
	status := 0
	for _, f := range files {
		tree, err := parse(gala.Tokens(f.Src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f.Name, err)
			status = 1
		}
		fmt.Printf("== %s\n%s\n", f.Name, gala.TreeString(tree))
	}
	os.Exit(status)
}
