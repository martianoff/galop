// Package repair checks galop's parsers against ANTLR on Repair.g4, a grammar
// whose decisions make ANTLR's SLL prediction fail where LL prediction finds
// the parse. ANTLR's answer there is the LL parse; galop finds it by
// repairing the failed SLL parse (or, for an entry rule without EOF, by
// parsing again in LL mode).
package repair

import (
	"fmt"
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
// they need no expected results; at least needLL of them must be inputs
// ANTLR accepts only with LL prediction, the ones a parse has to repair.
var cases = []struct {
	rule   string
	inputs []string
	needLL int
}{
	// p a a: SLL takes both a's into x and fails; LL leaves one for s1.
	// r a a a: no alternative is viable at x's conflict, so LL fails there.
	{"s1", []string{"p a a", "r a a", "r a", "p a", "p a a a", "r a a a", "p", "r"}, 1},
	{"s2", []string{"p b q", "r b q", "p " + parens(3) + " q", "r " + parens(3) + " q",
		"p " + parens(70) + " q", "r " + parens(70) + " q", "p " + parens(70) + " q q", "r " + parens(70)}, 3},
	// Every p item and q item needs a repair pass of its own.
	{"s3", []string{"p a a ; r a a ; p a a ;", "p a a ; p a a ; p a a ; p a a ;", "r a ; p a a ; q b q ; q " + parens(70) + " q ;",
		"p a a ; r a a a ;", "p a a ; p a ;", "q b q ;", "k b ; q " + parens(70) + " q ;", "q b q ; k b ; q b q ; p a a ;"}, 6},
	{"s4", []string{"p a a", "r a a", "p a a b", "p a"}, 1},
	// c x y z needs a as A2; c x y y z is rejected by both.
	{"s5", []string{"c x y z", "c x y y z", "x y z", "x y y z", "c x z", "c x"}, 1},
	// Both a's need A2: two repair passes.
	{"s6", []string{"c x y z x y z", "c x z x y z", "c x y z x z", "c x y z x y y z"}, 2},
	{"s7", []string{"x", "x x"}, 0},
	{"s8", []string{"c x y z", "x y y z", "c x z"}, 1},
	// Over groups the parse would stop after x; the tokens say otherwise.
	{"s9", []string{"x ( ( ( ( b ) ) ) )", "x ( b )", "x", "x ( ( q ) )"}, 0},
	// The first pass guesses that l10 goes on; c inputs need it to stop.
	{"s10", []string{"c a x " + parens(3) + " x " + parens(3) + " q", "c a x " + parens(3) + " q", "r a x " + parens(3) + " x " + parens(3),
		"c a x " + parens(3) + " x " + parens(3) + " x " + parens(3) + " q", "c a q", "c a x " + parens(3) + " x " + parens(3), "r a x"}, 0},
	// Twenty items, each a wrong guess: past the search's replays.
	{"s11", []string{strings.Repeat("c a x " + parens(3) + " x " + parens(3) + " q ; ", 20), strings.Repeat("c a x " + parens(3) + " q ; ", 3) + "c a q ;"}, 0},
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
			antlrErr := antlrFirstError(c.rule, src)
			// The repair against ANTLR: the verdict and, on success, the tree;
			// on failure, ANTLR's first error. And against the LL rerun: the same
			// error and (in the antlr flavor) the token stream left at the same place.
			check := func(flavor, got, where string, err error, rerunWhere string) {
				switch {
				case !wantOK && err != nil && err.Error() != antlrErr:
					t.Errorf("%s %q, %s flavor:\n error %v\n ANTLR %s", c.rule, src, flavor, err, antlrErr)
				case wantOK != (err == nil):
					t.Errorf("%s %q, %s flavor: ANTLR accepts=%v, galop error=%v", c.rule, src, flavor, wantOK, err)
				case wantOK && got != want:
					t.Errorf("%s %q, %s flavor:\n got  %s\n want %s", c.rule, src, flavor, got, want)
				case !wantOK && where != rerunWhere:
					t.Errorf("%s %q, %s flavor: repair stops at %s, the LL rerun at %s", c.rule, src, flavor, where, rerunWhere)
				}
			}
			n := rn.Entries[c.rule]
			got, err := n.Parse(rn.RepairLex(src))
			_, rerr := n.Rerun(rn.RepairLex(src))
			check("native", got, errText(err), err, errText(rerr))

			g := ra.GalopEntries[c.rule]
			tree, where, gerr := galop(src, g.Parse)
			_, rerunWhere, _ := galop(src, g.Rerun)
			galopTree := ""
			if gerr == nil {
				galopTree = antlr.TreesStringTree(tree, ruleNames, nil)
			}
			check("antlr", galopTree, where, gerr, rerunWhere)
		}
		if needLL < c.needLL {
			t.Errorf("%s: %d inputs need LL prediction, want at least %d", c.rule, needLL, c.needLL)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// galop parses src with an entry of galop's antlr flavor; where is the error
// and the token stream's index after it.
func galop(src string, parse func(*ra.RepairGalopParser) (antlr.Tree, error)) (antlr.Tree, string, error) {
	stream := antlr.NewCommonTokenStream(ra.NewRepairGalopLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel)
	tree, err := parse(ra.NewRepairGalopParser(stream))
	if err != nil {
		return nil, fmt.Sprintf("%v, stream at %d", err, stream.Index()), err
	}
	return tree, "", nil
}

// antlrParse parses src as rule with ANTLR in one prediction mode; ok says
// there was no syntax error.
func antlrParse(rule, src string, mode int) (string, bool) {
	p := ra.NewRepairParser(antlr.NewCommonTokenStream(ra.NewRepairLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	errs := &firstError{}
	p.AddErrorListener(errs)
	p.GetInterpreter().SetPredictionMode(mode)
	return antlr.TreesStringTree(ra.Entries[rule](p), p.GetRuleNames(), nil), errs.msg == ""
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

// antlrFirstError is the first syntax error ANTLR reports parsing src as rule
// the way the GALA compiler parses: SLL prediction, then, after an error, LL
// prediction from the start. It is empty when there is none.
func antlrFirstError(rule, src string) string {
	var msg string
	for _, mode := range []int{antlr.PredictionModeSLL, antlr.PredictionModeLL} {
		p := ra.NewRepairParser(antlr.NewCommonTokenStream(ra.NewRepairLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel))
		p.RemoveErrorListeners()
		errs := &firstError{}
		p.AddErrorListener(errs)
		p.GetInterpreter().SetPredictionMode(mode)
		ra.Entries[rule](p)
		if msg = errs.msg; msg == "" {
			return ""
		}
	}
	return msg
}
