// Command difftest parses every input file with two parsers and compares the
// trees ANTLR's ToStringTree prints for them. It reports the first differing
// subtree of each mismatch and any syntax errors, ends with "N/M identical",
// and exits 1 on any mismatch.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/diff"
	"github.com/martianoff/galop/differential/gala"
)

func main() {
	var src corpus.Source
	src.RegisterFlags(flag.CommandLine)
	a := flag.String("a", "antlr-lenient", "first parser: "+gala.ParserNames())
	b := flag.String("b", "antlr-ll", "second parser")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: difftest [-a name] [-b name] [-dir dir | -manifest file | file...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	src.Files = flag.Args()

	parsers := gala.Parsers()
	pa, okA := parsers[*a]
	pb, okB := parsers[*b]
	if !okA || !okB {
		fmt.Fprintf(os.Stderr, "difftest: unknown parser (have %s)\n", gala.ParserNames())
		os.Exit(2)
	}
	files, err := src.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "difftest:", err)
		os.Exit(2)
	}
	if diff.Report(os.Stdout, *a, *b, diff.Run(files, pa, pb)) > 0 {
		os.Exit(1)
	}
}
