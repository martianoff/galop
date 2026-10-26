// Package repair checks galop's parsers against ANTLR on Repair.g4, a grammar
// whose decisions make ANTLR's SLL prediction fail where LL prediction finds
// the parse. ANTLR's answer there is the LL parse; galop finds it by
// repairing the failed SLL parse (or, for an entry rule without EOF, by
// parsing again in LL mode).
package repair

import (
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	ra "github.com/martianoff/galop/differential/repair/antlr"
	rn "github.com/martianoff/galop/differential/repair/native"
)

// parens nests n in k pairs of parentheses, deeper than the static
// lookahead automaton follows.
func parens(k int) string {
	return strings.Repeat("( ", k) + "b" + strings.Repeat(" )", k)
}

// cases are inputs per entry rule. Each one is checked against ANTLR, so
// they need no expected results; sllFails counts how many must make ANTLR's
// SLL parse fail, so the repair is exercised.
var cases = []struct {
	rule     string
	inputs   []string
	sllFails int
}{
	// p a a: SLL takes both a's into x and fails; LL leaves one for s1.
	// r a a a: no alternative is viable at x's conflict, so LL fails there.
	{"s1", []string{"p a a", "r a a", "r a", "p a", "p a a a", "r a a a", "p", "r"}, 1},
	{"s2", []string{"p b q", "r b q", "p " + parens(3) + " q", "r " + parens(3) + " q",
		"p " + parens(70) + " q", "r " + parens(70) + " q", "p " + parens(70) + " q q", "r " + parens(70)}, 3},
	// Every p item and q item needs a repair pass of its own.
	{"s3", []string{"p a a ; r a a ; p a a ;", "p a a ; p a a ; p a a ; p a a ;", "r a ; p a a ; q b q ; q " + parens(70) + " q ;",
		"p a a ; r a a a ;", "p a a ; p a ;"}, 3},
	{"s4", []string{"p a a", "r a a", "p a a b", "p a"}, 1},
}

func TestRepairMatchesANTLR(t *testing.T) {
	for _, c := range cases {
		sllFails := 0
		for _, src := range c.inputs {
			if _, ok := antlrParse(c.rule, src, antlr.PredictionModeSLL); !ok {
				sllFails++
			}
			want, wantOK := antlrParse(c.rule, src, antlr.PredictionModeSLL)
			if !wantOK {
				want, wantOK = antlrParse(c.rule, src, antlr.PredictionModeLL)
			}
			for flavor, parse := range map[string]func(rule, src string) (string, error){"native": nativeParse, "antlr": galopParse} {
				got, err := parse(c.rule, src)
				switch {
				case wantOK != (err == nil):
					t.Errorf("%s %q, %s flavor: ANTLR accepts=%v, galop error=%v", c.rule, src, flavor, wantOK, err)
				case wantOK && got != want:
					t.Errorf("%s %q, %s flavor:\n got  %s\n want %s", c.rule, src, flavor, got, want)
				}
			}
		}
		if sllFails < c.sllFails {
			t.Errorf("%s: SLL fails on %d inputs, want at least %d", c.rule, sllFails, c.sllFails)
		}
	}
}

type errCount struct {
	antlr.DefaultErrorListener
	n int
}

func (e *errCount) SyntaxError(antlr.Recognizer, any, int, int, string, antlr.RecognitionException) {
	e.n++
}

// antlrParse parses src as rule with ANTLR in one prediction mode; ok says
// there was no syntax error.
func antlrParse(rule, src string, mode int) (string, bool) {
	p := ra.NewRepairParser(antlr.NewCommonTokenStream(ra.NewRepairLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	errs := &errCount{}
	p.AddErrorListener(errs)
	p.GetInterpreter().SetPredictionMode(mode)
	var tree antlr.Tree
	switch rule {
	case "s1":
		tree = p.S1()
	case "s2":
		tree = p.S2()
	case "s3":
		tree = p.S3()
	case "s4":
		tree = p.S4()
	}
	return antlr.TreesStringTree(tree, p.GetRuleNames(), nil), errs.n == 0
}

// nativeParse parses src as rule with galop's native flavor.
func nativeParse(rule, src string) (string, error) {
	ts := rn.RepairLex(src)
	var tree interface{ ToStringTree() string }
	var err error
	switch rule {
	case "s1":
		tree, err = rn.ParseS1(ts)
	case "s2":
		tree, err = rn.ParseS2(ts)
	case "s3":
		tree, err = rn.ParseS3(ts)
	case "s4":
		tree, err = rn.ParseS4(ts)
	}
	if err != nil {
		return "", err
	}
	return tree.ToStringTree(), nil
}

// galopParse parses src as rule with galop's antlr flavor.
func galopParse(rule, src string) (string, error) {
	p := ra.NewRepairGalopParser(antlr.NewCommonTokenStream(ra.NewRepairGalopLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
	var tree antlr.Tree
	var err error
	switch rule {
	case "s1":
		tree, err = p.S1()
	case "s2":
		tree, err = p.S2()
	case "s3":
		tree, err = p.S3()
	case "s4":
		tree, err = p.S4()
	}
	if err != nil {
		return "", err
	}
	ra.RepairParserInit()
	return antlr.TreesStringTree(tree, ra.RepairParserStaticData.RuleNames, nil), nil
}
