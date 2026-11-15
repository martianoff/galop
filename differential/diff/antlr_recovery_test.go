package diff

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/mutate"
)

// observed is everything an error listener sees of a parser, ANTLR's or the
// antlr flavor's: each error's position, message, offending token and
// exception, and the parser's expected tokens, rule stack and context (its
// type and number of children) when the error is reported. ATN state
// numbers are left out: the antlr flavor's are galop's.
type observed struct {
	*antlr.DefaultErrorListener
	reports []string
}

func (l *observed) SyntaxError(rec antlr.Recognizer, offending interface{}, line, column int, msg string, e antlr.RecognitionException) {
	p := rec.(antlr.Parser)
	ctx := p.GetParserRuleContext()
	var offendingOfE string
	// A NoViableAltException's GetOffendingToken is nil in ANTLR's Go runtime.
	if e != nil && e.GetOffendingToken() != nil {
		offendingOfE = fmt.Sprint(e.GetOffendingToken().GetTokenIndex())
	}
	var expected []int
	for _, iv := range p.GetExpectedTokens().GetIntervals() {
		for t := iv.Start; t < iv.Stop; t++ {
			expected = append(expected, t)
		}
	}
	l.reports = append(l.reports, fmt.Sprintf("%d:%d: %s | token %d | %T %s | expected %v | stack %v | %T with %d children",
		line, column, msg, offending.(antlr.Token).GetTokenIndex(), e, offendingOfE, expected,
		p.GetRuleInvocationStack(nil), ctx, ctx.GetChildCount()))
}

// dumpTree prints every node of an ANTLR tree: each context's type, start
// and stop tokens and exception, each token's index and text, error nodes
// marked.
func dumpTree(t antlr.Tree) string {
	var b strings.Builder
	var walk func(antlr.Tree, string)
	walk = func(t antlr.Tree, ind string) {
		switch n := t.(type) {
		case antlr.ErrorNode:
			fmt.Fprintf(&b, "%serror %d %q\n", ind, n.GetSymbol().GetTokenIndex(), n.GetText())
		case antlr.TerminalNode:
			fmt.Fprintf(&b, "%stoken %d %q\n", ind, n.GetSymbol().GetTokenIndex(), n.GetText())
		case antlr.ParserRuleContext:
			stop := -2
			if n.GetStop() != nil {
				stop = n.GetStop().GetTokenIndex()
			}
			fmt.Fprintf(&b, "%s%T %d..%d exception %s\n", ind, n, n.GetStart().GetTokenIndex(), stop, exceptionType(n))
			for _, c := range n.GetChildren() {
				walk(c, ind+"  ")
			}
		}
	}
	walk(t, "")
	return b.String()
}

// exceptionType is the type of a context's exception field, which ANTLR's
// Go runtime sets but has no getter for, or "<nil>".
func exceptionType(ctx antlr.ParserRuleContext) string {
	v := reflect.ValueOf(ctx)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	f := v.FieldByName("exception")
	if !f.IsValid() || f.IsNil() {
		return "<nil>"
	}
	return f.Elem().Type().String()
}

// flavorOutcome is what one parse gave: the listener's reports and the tree.
type flavorOutcome struct {
	reports []string
	tree    string
}

// recoverByANTLR parses src with ANTLR's parser as the GALA compiler does on
// a file with an error; recoverByFlavor with the antlr flavor's SourceFile,
// as a drop-in for it.
func recoverByANTLR(src string) flavorOutcome {
	l := &observed{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	ctx, stream := gala.ParseRecoveredWith(src, l)
	return flavorOutcome{l.reports, dumpTree(ctx) + fmt.Sprintf("stream at %d\n", stream.Index())}
}

func recoverByFlavor(src string) flavorOutcome {
	l := &observed{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	stream := gala.GalopTokens(src)
	p := antlrgala.NewgalaGalopParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(l)
	ctx := p.SourceFile()
	return flavorOutcome{l.reports, dumpTree(ctx) + fmt.Sprintf("stream at %d\n", stream.Index())}
}

// TestANTLRFlavorRecoversAsANTLR compares the antlr flavor's SourceFile with
// ANTLR's on the corpus's rejected mutants and editor-style edits: every call
// of the error listener, with what it reads of the parser, and the
// error-recovered tree, node by node.
func TestANTLRFlavorRecoversAsANTLR(t *testing.T) {
	srcs := mutationSources(t)
	for _, kind := range []struct {
		name   string
		n      int
		change func(*rand.Rand, string) string
	}{
		{"mutants", *recoveryMutants, mutate.Mutate},
		{"edits", *recoveryEdits, editLike},
	} {
		inputs := rejectedCopies(rand.New(rand.NewSource(*mutantSeed)), srcs, kind.change, kind.n)
		want := make([]flavorOutcome, len(inputs))
		parallel(inputs, func(i int, src string) { want[i] = recoverByANTLR(src) })
		same, errors, shown := 0, 0, 0
		for i, src := range inputs {
			got := recoverByFlavor(src)
			errors += len(want[i].reports)
			if strings.Join(got.reports, "\n") == strings.Join(want[i].reports, "\n") && got.tree == want[i].tree {
				same++
				continue
			}
			if shown < 5 {
				t.Errorf("%s %d:\n the listener sees\n  %s\n ANTLR's sees\n  %s\n tree same: %v\n%s", kind.name, i,
					strings.Join(got.reports, "\n  "), strings.Join(want[i].reports, "\n  "), got.tree == want[i].tree, src)
			}
			shown++
		}
		if len(inputs) == 0 {
			t.Fatalf("%s: no rejected input was compared", kind.name)
		}
		t.Logf("%s: %d inputs (%d errors), %d recovered as ANTLR recovers", kind.name, len(inputs), errors, same)
	}
}
