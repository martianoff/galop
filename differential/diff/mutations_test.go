package diff

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
)

var (
	mutants    = flag.Int("mutants", 200, "TestMutations: how many mutated files to check")
	mutantSeed = flag.Int64("mutant_seed", 1, "TestMutations: the random seed")
)

// insertions are text TestMutations puts into a file at a token boundary:
// stray tokens, and constructs whose decisions need full-context prediction
// (a tuple pattern or a lambda, `if x {`, newline-separated calls).
var insertions = []string{
	"(", ")", "=>", "int", "x", ".", ",", "=", "{", "}", "[", "]", ":", "...", "func ", "return\n",
	"(a, b) => ", "case (a, b) => a\n", "if x {", "x {", "\n(", "\n-", "f(a)", "Foo{a: 1}",
}

// statements are valid statements TestMutations puts on a line of its own
// inside a function body, so that the file most likely stays valid but needs
// the parse ANTLR's LL prediction gives.
var statements = []string{
	"val zq = pair match { case (a, b) => a }",
	"val zq = pair match {\n case (a, b) => a + b\n case _ => 0\n }",
	"if zq { }",
	"if zq {\n zq\n }",
	"zq(1)\n(a, b) => a",
	"val zq = f(a)\n-b",
	"val zq = xs.Map((a) => a)",
	"val zq = Point{x: 1}",
	"val zq = if (a) b else c.d",
	"return",
}

// TestMutations compares galop's parsers with ANTLR on mutated corpus files:
// a token dropped or doubled, text inserted, or a statement added. Most
// mutants are rejected; the rest are where SLL prediction and LL prediction
// part ways, which is what the repair of a failed SLL parse has to get right.
// Both flavors must agree with ANTLR on whether the input parses and, if it
// does, on the tree. -mutants and -mutant_seed size the run.
func TestMutations(t *testing.T) {
	var srcs []string
	for _, f := range files(t) {
		// Large files make ANTLR's LL pass slow without adding much.
		if len(f.Src) < 20000 {
			srcs = append(srcs, gala.StripBOM(f.Src))
		}
	}
	// Each flavor parses with the repair of a failed SLL parse (rerun false)
	// and with ANTLR's whole-input LL rerun (rerun true). On error, where says
	// where the parse left off: the error text, with its position, and for the
	// antlr flavor the token stream's index.
	flavors := []struct {
		name  string
		parse func(src string, rerun bool) (tree, where string, err error)
	}{
		{"native", func(src string, rerun bool) (string, string, error) {
			parse := nativegala.Parse
			if rerun {
				parse = nativegala.ParseByRerun
			}
			root, err := parse(src)
			if err != nil {
				return "", err.Error(), err
			}
			return root.ToStringTree(), "", nil
		}},
		{"antlr flavor", func(src string, rerun bool) (string, string, error) {
			stream := gala.GalopTokens(src)
			var ctx antlr.ParserRuleContext
			var err error
			if rerun {
				ctx, err = antlrgala.ParseGalopByRerun(stream)
			} else {
				ctx, err = antlrgala.NewgalaGalopParser(stream).SourceFile()
			}
			if err != nil {
				return "", fmt.Sprintf("%v, stream at %d", err, stream.Index()), err
			}
			return gala.TreeString(ctx), "", nil
		}},
	}
	r := rand.New(rand.NewSource(*mutantSeed))
	accepted, differ := 0, 0
	for i := 0; i < *mutants; i++ {
		src := mutate(r, srcs[r.Intn(len(srcs))])
		want, wantOK := antlrVerdict(src)
		if wantOK {
			accepted++
		}
		for _, f := range flavors {
			got, where, err := f.parse(src, false)
			switch {
			case wantOK != (err == nil):
				differ++
				t.Errorf("mutant %d: ANTLR accepts=%v, %s error=%v\n%s", i, wantOK, f.name, err, src)
			case wantOK && got != want:
				differ++
				t.Errorf("mutant %d: %s tree differs\n%s", i, f.name, firstDifference(want, got))
			case !wantOK:
				// Rejected: the error and where the parse stopped must be the
				// rerun's, not just the verdict.
				if _, rerunWhere, _ := f.parse(src, true); where != rerunWhere {
					differ++
					t.Errorf("mutant %d: %s error differs from the LL rerun's:\n repair: %s\n rerun:  %s", i, f.name, where, rerunWhere)
				}
			}
		}
	}
	t.Logf("%d mutants, %d accepted by ANTLR, %d differences", *mutants, accepted, differ)
}

// mutate changes src at a random token or line.
func mutate(r *rand.Rand, src string) string {
	if r.Intn(3) == 0 {
		if s, ok := addStatement(r, src); ok {
			return s
		}
	}
	// The last token is EOF, which has no text to change.
	toks := nativegala.Tokens(src).Tokens
	if len(toks) < 2 {
		return src
	}
	tok := toks[r.Intn(len(toks)-1)]
	off, end := int(tok.Start), int(tok.End)
	switch r.Intn(3) {
	case 0:
		return src[:off] + src[end:]
	case 1:
		return src[:end] + " " + src[off:]
	default:
		return src[:off] + insertions[r.Intn(len(insertions))] + " " + src[off:]
	}
}

// addStatement puts one of statements before an indented `val`, `var` or
// `Println(` line, at that line's indentation.
func addStatement(r *rand.Rand, src string) (string, bool) {
	lines := strings.Split(src, "\n")
	var at []int
	for i, l := range lines {
		body := strings.TrimLeft(l, " \t")
		if len(l)-len(body) >= 4 && (strings.HasPrefix(body, "val ") || strings.HasPrefix(body, "var ") || strings.HasPrefix(body, "Println(")) {
			at = append(at, i)
		}
	}
	if len(at) == 0 {
		return "", false
	}
	i := at[r.Intn(len(at))]
	indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
	stmt := indent + strings.ReplaceAll(statements[r.Intn(len(statements))], "\n", "\n"+indent)
	return strings.Join(append(lines[:i:i], append([]string{stmt}, lines[i:]...)...), "\n"), true
}
