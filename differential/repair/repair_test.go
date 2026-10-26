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

// entry is an entry rule in ANTLR's parser and in galop's two flavors.
type entry struct {
	antlr  func(*ra.RepairParser) antlr.Tree
	galop  func(*ra.RepairGalopParser) (antlr.Tree, error)
	native func(*rn.RepairTokens) (string, error)
}

// tree adapts a native entry point to return the tree's string form.
func tree[N interface{ ToStringTree() string }](parse func(*rn.RepairTokens) (N, error)) func(*rn.RepairTokens) (string, error) {
	return func(ts *rn.RepairTokens) (string, error) {
		n, err := parse(ts)
		if err != nil {
			return "", err
		}
		return n.ToStringTree(), nil
	}
}

var (
	s1 = entry{func(p *ra.RepairParser) antlr.Tree { return p.S1() }, func(p *ra.RepairGalopParser) (antlr.Tree, error) { return p.S1() }, tree(rn.ParseS1)}
	s2 = entry{func(p *ra.RepairParser) antlr.Tree { return p.S2() }, func(p *ra.RepairGalopParser) (antlr.Tree, error) { return p.S2() }, tree(rn.ParseS2)}
	s3 = entry{func(p *ra.RepairParser) antlr.Tree { return p.S3() }, func(p *ra.RepairGalopParser) (antlr.Tree, error) { return p.S3() }, tree(rn.ParseS3)}
	s4 = entry{func(p *ra.RepairParser) antlr.Tree { return p.S4() }, func(p *ra.RepairGalopParser) (antlr.Tree, error) { return p.S4() }, tree(rn.ParseS4)}
)

// cases are inputs per entry rule. Each one is checked against ANTLR, so
// they need no expected results; at least `needLL` of them must be inputs
// ANTLR accepts only with LL prediction, the ones a parse has to repair.
var cases = []struct {
	name   string
	rule   entry
	inputs []string
	needLL int
}{
	// p a a: SLL takes both a's into x and fails; LL leaves one for s1.
	// r a a a: no alternative is viable at x's conflict, so LL fails there.
	{"s1", s1, []string{"p a a", "r a a", "r a", "p a", "p a a a", "r a a a", "p", "r"}, 1},
	{"s2", s2, []string{"p b q", "r b q", "p " + parens(3) + " q", "r " + parens(3) + " q",
		"p " + parens(70) + " q", "r " + parens(70) + " q", "p " + parens(70) + " q q", "r " + parens(70)}, 3},
	// Every p item and q item needs a repair pass of its own.
	{"s3", s3, []string{"p a a ; r a a ; p a a ;", "p a a ; p a a ; p a a ; p a a ;", "r a ; p a a ; q b q ; q " + parens(70) + " q ;",
		"p a a ; r a a a ;", "p a a ; p a ;", "q b q ;", "k b ; q " + parens(70) + " q ;", "q b q ; k b ; q b q ; p a a ;"}, 6},
	{"s4", s4, []string{"p a a", "r a a", "p a a b", "p a"}, 1},
}

func TestRepairMatchesANTLR(t *testing.T) {
	ra.RepairParserInit()
	ruleNames := ra.RepairParserStaticData.RuleNames
	for _, c := range cases {
		needLL := 0
		for _, src := range c.inputs {
			want, wantOK := antlrParse(c.rule, src, antlr.PredictionModeSLL)
			if !wantOK {
				want, wantOK = antlrParse(c.rule, src, antlr.PredictionModeLL)
				if wantOK {
					needLL++
				}
			}
			check := func(flavor, got string, err error) {
				switch {
				case wantOK != (err == nil):
					t.Errorf("%s %q, %s flavor: ANTLR accepts=%v, galop error=%v", c.name, src, flavor, wantOK, err)
				case wantOK && got != want:
					t.Errorf("%s %q, %s flavor:\n got  %s\n want %s", c.name, src, flavor, got, want)
				}
			}
			got, err := c.rule.native(rn.RepairLex(src))
			check("native", got, err)
			p := ra.NewRepairGalopParser(antlr.NewCommonTokenStream(ra.NewRepairGalopLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
			if tr, err := c.rule.galop(p); err != nil {
				check("antlr", "", err)
			} else {
				check("antlr", antlr.TreesStringTree(tr, ruleNames, nil), nil)
			}
		}
		if needLL < c.needLL {
			t.Errorf("%s: %d inputs need LL prediction, want at least %d", c.name, needLL, c.needLL)
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
func antlrParse(rule entry, src string, mode int) (string, bool) {
	p := ra.NewRepairParser(antlr.NewCommonTokenStream(ra.NewRepairLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	errs := &errCount{}
	p.AddErrorListener(errs)
	p.GetInterpreter().SetPredictionMode(mode)
	return antlr.TreesStringTree(rule.antlr(p), p.GetRuleNames(), nil), errs.n == 0
}
