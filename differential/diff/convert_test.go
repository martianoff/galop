package diff

import (
	"flag"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/mutate"
	"github.com/martianoff/galop/differential/nativegala"
)

var recoveredTrees = flag.Int("recovered_trees", 100, "TestConvertRecoveredTrees: how many rejected mutants to convert")

// converted parses src with ANTLR (the GALA compiler's strategy, with ANTLR's
// error recovery) and with galop's antlr flavor, and converts both trees to
// the native tree. The antlr flavor stops at the first syntax error, so its
// tree is nil then.
func converted(src string) (antlrTree, flavorTree *nativegala.GalaTree, antlrCtx antlr.ParserRuleContext, antlrErr error) {
	src = gala.StripBOM(src)
	stream := gala.Tokens(src)
	antlrCtx, antlrErr = gala.Lenient(stream)
	antlrTree, _ = antlrgala.GalaToNative(antlrCtx, stream, src)
	fstream := gala.GalopTokens(src)
	if ctx, err := antlrgala.NewgalaGalopParser(fstream).SourceFile(); err == nil {
		flavorTree, _ = antlrgala.GalaToNative(ctx, fstream, src)
	}
	return antlrTree, flavorTree, antlrCtx, antlrErr
}

// recovered calls check on n mutants that lex but ANTLR rejects, with
// ANTLR's error-recovered tree of each and its conversion.
func recovered(tb testing.TB, n int, check func(i int, src string, tree *nativegala.GalaTree, ctx antlr.ParserRuleContext)) {
	srcs := mutationSources(tb)
	r := rand.New(rand.NewSource(*mutantSeed))
	i := 0
	for tries := 0; i < n && tries < 50*n; tries++ {
		src := mutate.Mutate(r, srcs[r.Intn(len(srcs))])
		if len(nativegala.Tokens(src).Errors) > 0 {
			continue
		}
		// Only the mutants ANTLR rejects are converted.
		stream := gala.Tokens(src)
		if ctx, err := gala.Lenient(stream); err != nil {
			tree, _ := antlrgala.GalaToNative(ctx, stream, src)
			i++
			check(i, src, tree, ctx)
		}
	}
	if i == 0 {
		tb.Fatal("no mutant was rejected, so no recovered tree was converted")
	}
}

// sameTree describes the first difference between two native trees, or "".
func sameTree(want, got *nativegala.GalaTree) string {
	switch {
	case !reflect.DeepEqual(want.Tokens, got.Tokens):
		return "tokens differ"
	case !reflect.DeepEqual(want.Hidden, got.Hidden):
		return "hidden tokens differ"
	case len(want.Nodes) != len(got.Nodes):
		return fmt.Sprintf("%d nodes, want %d", len(got.Nodes), len(want.Nodes))
	case !slices.Equal(want.Kids, got.Kids):
		return "child references differ"
	case !slices.Equal(want.TokenLabels, got.TokenLabels):
		return "token labels differ"
	case want.Next != got.Next:
		return fmt.Sprintf("next token %d, want %d", got.Next, want.Next)
	case len(got.ErrorKids)+len(got.Conjured) > 0:
		return "error tokens in a tree native parsing accepts"
	}
	for i := range want.Nodes {
		if want.Nodes[i] != got.Nodes[i] {
			return fmt.Sprintf("node %d is %+v, want %+v", i, got.Nodes[i], want.Nodes[i])
		}
	}
	return ""
}

// TestConvertMatchesNative converts ANTLR's tree and galop's antlr-flavor
// tree of every input native parsing accepts, and requires the native tree:
// the same tokens, nodes (rule, labeled alternative, label, span), child
// references and token labels.
func TestConvertMatchesNative(t *testing.T) {
	checked := 0
	for _, f := range files(t) {
		root, err := nativegala.Parse(f.Src)
		if err != nil {
			continue
		}
		checked++
		antlrTree, flavorTree, _, antlrErr := converted(f.Src)
		if antlrErr != nil {
			t.Errorf("%s: native parsing accepts, ANTLR reports %v", f.Name, antlrErr)
			continue
		}
		if d := sameTree(root.Tree(), antlrTree); d != "" {
			t.Errorf("%s: converted ANTLR tree: %s", f.Name, d)
		}
		if flavorTree == nil {
			t.Errorf("%s: galop's antlr flavor rejects it", f.Name)
		} else if d := sameTree(root.Tree(), flavorTree); d != "" {
			t.Errorf("%s: converted antlr-flavor tree: %s", f.Name, d)
		}
	}
	t.Logf("%d inputs converted and compared", checked)
}

// TestConvertRecoveredTrees converts ANTLR's error-recovered trees of
// rejected mutants and checks they are well formed: children before their
// parents, every node but the root a child once, real tokens in order and
// at most once, error tokens marked in order, spans in bounds, and ANTLR's
// own rendering of the tree.
func TestConvertRecoveredTrees(t *testing.T) {
	withErrors := 0
	recovered(t, *recoveredTrees, func(i int, src string, tree *nativegala.GalaTree, ctx antlr.ParserRuleContext) {
		if msg := wellFormed(tree); msg != "" {
			t.Errorf("mutant %d: %s\n%s", i, msg, src)
			return
		}
		if got, want := tree.Root().ToStringTree(), gala.TreeString(ctx); got != want {
			t.Errorf("mutant %d: the converted tree prints differently\n%s", i, firstDifference(want, got))
		}
		if len(tree.ErrorKids) > 0 {
			withErrors++
		}
	})
	t.Logf("%d recovered trees converted, %d with error tokens", *recoveredTrees, withErrors)
}

// wellFormed describes the first way tree breaks the native tree's
// invariants, or "".
func wellFormed(tree *nativegala.GalaTree) string {
	nTokens := len(tree.Tokens) + len(tree.Conjured)
	parents := make([]int, len(tree.Nodes))
	for i := range parents {
		parents[i] = -1
	}
	for i, n := range tree.Nodes {
		if n.Stop < n.Start-1 || n.Start < 0 || int(n.Start) > len(tree.Tokens) || int(n.Stop) >= len(tree.Tokens) {
			return fmt.Sprintf("node %d spans %d..%d of %d tokens", i, n.Start, n.Stop, len(tree.Tokens))
		}
		if n.Kids < 0 || int(n.Kids+n.NKids) > len(tree.Kids) {
			return fmt.Sprintf("node %d's children are out of range", i)
		}
		for _, k := range tree.Kids[n.Kids : n.Kids+n.NKids] {
			switch {
			case k >= int32(i):
				return fmt.Sprintf("node %d has child %d, which does not come before it", i, k)
			case k >= 0 && parents[k] >= 0:
				return fmt.Sprintf("node %d is a child of %d and %d", k, parents[k], i)
			case k >= 0:
				parents[k] = i
			case int(-k-1) >= nTokens:
				return fmt.Sprintf("node %d has token child %d of %d", i, -k-1, nTokens)
			}
		}
	}
	for i, p := range parents[:len(parents)-1] {
		if p < 0 {
			return fmt.Sprintf("node %d has no parent", i)
		}
	}
	for i, p := range tree.ErrorKids {
		switch {
		case p < 0 || int(p) >= len(tree.Kids) || tree.Kids[p] >= 0:
			return fmt.Sprintf("error token %d is child reference %d, not a token", i, p)
		case i > 0 && p <= tree.ErrorKids[i-1]:
			return "error tokens are out of order"
		}
	}
	// Real tokens, read off the tree in order, come in order and once each.
	last := -1
	return tokensInOrder(tree.Root(), len(tree.Tokens), &last)
}

// tokensInOrder checks that the real tokens of n's subtree come after *last
// and in order, moving *last to the last one.
func tokensInOrder(n nativegala.GalaNodeRef, real int, last *int) string {
	for i := 0; i < n.ChildCount(); i++ {
		c := n.Child(i)
		if !c.IsToken() {
			if msg := tokensInOrder(c.Node(), real, last); msg != "" {
				return msg
			}
			continue
		}
		if ti := c.Terminal().Index(); ti < real {
			if ti <= *last {
				return fmt.Sprintf("token %d comes after token %d", ti, *last)
			}
			*last = ti
		}
	}
	return ""
}

// events records what GalaWalk tells a listener, and checks that the kind
// methods pair with the EveryRule ones.
type events struct {
	nativegala.GalaBaseListener
	log  []string
	open []string
	bad  string
}

func (e *events) EnterEveryRule(n nativegala.GalaNodeRef) {
	e.log = append(e.log, fmt.Sprintf("enter %d", n.Index()))
}

func (e *events) ExitEveryRule(n nativegala.GalaNodeRef) {
	e.log = append(e.log, fmt.Sprintf("exit %d", n.Index()))
}

func (e *events) VisitTerminal(t nativegala.GalaTerminal) {
	e.log = append(e.log, fmt.Sprintf("token %d", t.Index()))
}

func (e *events) VisitErrorNode(t nativegala.GalaTerminal) {
	e.log = append(e.log, fmt.Sprintf("error %d", t.Index()))
}

func (e *events) EnterCaseClause(n nativegala.CaseClauseNode)     { e.enter("caseClause", n.Ref()) }
func (e *events) ExitCaseClause(n nativegala.CaseClauseNode)      { e.exit("caseClause", n.Ref()) }
func (e *events) EnterTypedPattern(n nativegala.TypedPatternNode) { e.enter("typedPattern", n.Ref()) }
func (e *events) ExitTypedPattern(n nativegala.TypedPatternNode)  { e.exit("typedPattern", n.Ref()) }

func (e *events) enter(kind string, n nativegala.GalaNodeRef) {
	if e.log[len(e.log)-1] != fmt.Sprintf("enter %d", n.Index()) {
		e.bad = fmt.Sprintf("Enter of %s %d does not follow its EnterEveryRule", kind, n.Index())
	}
	e.open = append(e.open, fmt.Sprintf("%s %d", kind, n.Index()))
}

func (e *events) exit(kind string, n nativegala.GalaNodeRef) {
	want := fmt.Sprintf("%s %d", kind, n.Index())
	if len(e.open) == 0 || e.open[len(e.open)-1] != want {
		e.bad = "Exit of " + want + " does not match the last Enter"
		return
	}
	e.open = e.open[:len(e.open)-1]
}

// wantEvents is what ANTLR's ParseTreeWalker reports for n's subtree.
func wantEvents(n nativegala.GalaNodeRef, log []string) []string {
	log = append(log, fmt.Sprintf("enter %d", n.Index()))
	for i := 0; i < n.ChildCount(); i++ {
		switch c := n.Child(i); {
		case c.IsError():
			log = append(log, fmt.Sprintf("error %d", c.Terminal().Index()))
		case c.IsToken():
			log = append(log, fmt.Sprintf("token %d", c.Terminal().Index()))
		default:
			log = wantEvents(c.Node(), log)
		}
	}
	return append(log, fmt.Sprintf("exit %d", n.Index()))
}

// kinds is a visitor that returns the handle type of the kinds it overrides.
type kinds struct{ nativegala.GalaBaseVisitor[string] }

func (kinds) VisitCaseClause(nativegala.CaseClauseNode) string     { return "CaseClauseNode" }
func (kinds) VisitTypedPattern(nativegala.TypedPatternNode) string { return "TypedPatternNode" }

// checkTyped checks the typed view of tree: Typed is the handle of each
// node's rule or labeled alternative, GalaAccept calls the visitor method for
// it, and GalaWalk reports nodes, tokens and error tokens in ANTLR's order.
func checkTyped(tree *nativegala.GalaTree) string {
	for i := range tree.Nodes {
		n := tree.Node(i)
		want := strings.TrimSuffix(strings.TrimSuffix(contextType(n), "Context"), "_") + "Node"
		if got := reflect.TypeOf(n.Typed()).Elem().Name(); got != want {
			return fmt.Sprintf("node %d is typed %s, want %s", i, got, want)
		}
		visited := ""
		if want == "CaseClauseNode" || want == "TypedPatternNode" {
			visited = want
		}
		if isClause, kind := want == "CaseClauseNode", n.Kind(); isClause != (kind == nativegala.GalaKIND_caseClause) ||
			(want == "TypedPatternNode") != (kind == nativegala.GalaKIND_typedPattern) {
			return fmt.Sprintf("node %d: Kind is %d for a %s", i, kind, want)
		}
		if v := nativegala.GalaAccept[string](n, kinds{}); v != visited {
			return fmt.Sprintf("node %d: Accept returned %q, want %q", i, v, visited)
		}
	}
	l := &events{}
	nativegala.GalaWalk(tree.Root(), l)
	switch {
	case l.bad != "":
		return l.bad
	case len(l.open) > 0:
		return l.open[0] + " is never exited"
	case !slices.Equal(l.log, wantEvents(tree.Root(), nil)):
		return "GalaWalk reports events out of order"
	}
	return ""
}

// TestTypedVisitorListener checks the typed view, visitor and listener on
// the native trees of the inputs and on converted recovered trees.
func TestTypedVisitorListener(t *testing.T) {
	for _, f := range files(t) {
		if root, err := nativegala.Parse(f.Src); err == nil {
			if msg := checkTyped(root.Tree()); msg != "" {
				t.Errorf("%s: %s", f.Name, msg)
			}
		}
	}
	recovered(t, 30, func(i int, src string, tree *nativegala.GalaTree, _ antlr.ParserRuleContext) {
		if msg := checkTyped(tree); msg != "" {
			t.Errorf("mutant %d: %s\n%s", i, msg, src)
		}
	})
}

// BenchmarkConvert converts galop's antlr-flavor trees of the inputs to the
// native tree (the trees are built once, outside the timing).
func BenchmarkConvert(b *testing.B) {
	type parsed struct {
		root   antlr.ParserRuleContext
		stream *antlr.CommonTokenStream
		src    string
	}
	var trees []parsed
	for _, f := range files(b) {
		src := gala.StripBOM(f.Src)
		stream := gala.GalopTokens(src)
		if root, err := antlrgala.NewgalaGalopParser(stream).SourceFile(); err == nil {
			trees = append(trees, parsed{root, stream, src})
		}
	}
	b.ResetTimer()
	for range b.N {
		for _, t := range trees {
			antlrgala.GalaToNative(t.root, t.stream, t.src)
		}
	}
}

// BenchmarkTyped calls Typed on every node of the inputs' trees, with a type
// switch on the result, after the typed views are built: it must not
// allocate.
func BenchmarkTyped(b *testing.B) {
	var trees []*nativegala.GalaTree
	nodes := 0
	for _, f := range files(b) {
		if root, err := nativegala.Parse(f.Src); err == nil {
			root.Typed()
			trees = append(trees, root.Tree())
			nodes += len(root.Tree().Nodes)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	clauses := 0
	for range b.N {
		for _, t := range trees {
			for i := range t.Nodes {
				switch c := t.Node(i).Typed().(type) {
				case *nativegala.CaseClauseNode:
					clauses += c.ChildCount()
				case *nativegala.TypedPatternNode:
					clauses++
				}
			}
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*nodes), "ns/node")
}

// BenchmarkTypedView builds the typed views of the inputs' trees: the
// one-time cost the first Typed call on each tree pays.
func BenchmarkTypedView(b *testing.B) {
	fs := files(b)
	b.ReportAllocs()
	for range b.N {
		b.StopTimer()
		var roots []nativegala.SourceFileNode
		for _, f := range fs {
			if root, err := nativegala.Parse(f.Src); err == nil {
				roots = append(roots, root)
			}
		}
		b.StartTimer()
		for _, r := range roots {
			r.Typed()
		}
	}
}

// BenchmarkKind switches on Kind for every node of the inputs' trees: the
// typed dispatch that needs no handles.
func BenchmarkKind(b *testing.B) {
	var trees []*nativegala.GalaTree
	nodes := 0
	for _, f := range files(b) {
		if root, err := nativegala.Parse(f.Src); err == nil {
			trees = append(trees, root.Tree())
			nodes += len(root.Tree().Nodes)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	clauses := 0
	for range b.N {
		for _, t := range trees {
			for i := range t.Nodes {
				switch n := t.Node(i); n.Kind() {
				case nativegala.GalaKIND_caseClause:
					clauses += nativegala.CaseClauseNode{GalaNodeRef: n}.ChildCount()
				case nativegala.GalaKIND_typedPattern:
					clauses++
				}
			}
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*nodes), "ns/node")
}
