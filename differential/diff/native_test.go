package diff

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"unicode"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
)

// TestNativeMatchesANTLR is the native flavor's acceptance test. On every
// corpus file it compares galop's native lexer and parser with ANTLR's lexer
// and parser, driven the way the GALA compiler drives them (line-break
// re-typing, SLL with an LL retry):
//
//   - tokens: every default-channel token's type, text, line and column, and
//     every off-channel token's type, channel, text, line, column and place;
//   - trees: node for node, the rule, the labeled alternative (ANTLR's context
//     type), the first and last token and the children in order, plus
//     ToStringTree;
//   - accessors: for a few rules, the typed getters against ANTLR's.
//
// The files ANTLR rejects must be rejected.
func TestNativeMatchesANTLR(t *testing.T) {
	fs := files(t)
	var mu sync.Mutex
	var problems []string
	parallel(fs, func(f corpus.File) {
		if p := compareNative(f); p != "" {
			mu.Lock()
			problems = append(problems, p)
			mu.Unlock()
		}
	})
	slices.Sort(problems)
	for i, p := range problems {
		if i == 10 {
			t.Errorf("... and %d more files differ", len(problems)-10)
			break
		}
		t.Error(p)
	}
	t.Logf("%d/%d corpus files give ANTLR's tokens and tree", len(fs)-len(problems), len(fs))
}

// compareNative compares the native flavor with ANTLR on one file and
// describes the first difference, or returns "".
func compareNative(f corpus.File) string {
	ts := nativegala.Tokens(f.Src)
	stream := gala.Tokens(f.Src)
	if d := compareTokens(f.Name, ts, stream); d != "" {
		return d
	}
	stream.Seek(0)
	ref, err := gala.Lenient(stream)
	root, nerr := nativegala.ParseSourceFile(ts)
	if _, skip := rejected[f.Name]; skip || err != nil {
		if nerr == nil {
			return f.Name + ": the native parser accepts a file ANTLR rejects"
		}
		return ""
	}
	if nerr != nil {
		return fmt.Sprintf("%s: the native parser rejects a file ANTLR accepts: %v", f.Name, nerr)
	}
	if d := compareNode(f.Name, ts, root.GalaNodeRef, ref); d != "" {
		return d + "\n" + firstDifference(gala.TreeString(ref), root.ToStringTree())
	}
	return ""
}

// compareTokens compares the native token list with ANTLR's token stream.
func compareTokens(name string, ts *nativegala.GalaTokens, stream *antlr.CommonTokenStream) string {
	stream.Fill()
	next := 0
	hidden := 0
	for _, a := range stream.GetAllTokens() {
		if a.GetChannel() != antlr.TokenDefaultChannel {
			if hidden >= len(ts.Hidden) {
				return fmt.Sprintf("%s: ANTLR has off-channel token %v, native has no more", name, a)
			}
			h := ts.Hidden[hidden]
			if d := sameToken(ts, h.GalaToken, a); d != "" || int(h.Channel) != a.GetChannel() || int(h.Next) != next {
				return fmt.Sprintf("%s: off-channel token %d: native %+v %q, ANTLR %v %s", name, hidden, h, h.Text(ts.Src), a, d)
			}
			hidden++
			continue
		}
		if next >= len(ts.Tokens) {
			return fmt.Sprintf("%s: ANTLR has token %v, native has no more", name, a)
		}
		if d := sameToken(ts, ts.Tokens[next], a); d != "" {
			return fmt.Sprintf("%s: token %d: %s", name, next, d)
		}
		next++
	}
	if next != len(ts.Tokens) || hidden != len(ts.Hidden) {
		return fmt.Sprintf("%s: native has %d+%d tokens, ANTLR %d+%d", name, len(ts.Tokens), len(ts.Hidden), next, hidden)
	}
	return ""
}

// sameToken compares a native token with an ANTLR token: type, text, line, column.
func sameToken(ts *nativegala.GalaTokens, n nativegala.GalaToken, a antlr.Token) string {
	if int(n.Type) != a.GetTokenType() || n.Text(ts.Src) != a.GetText() || int(n.Line) != a.GetLine() || int(n.Column) != a.GetColumn() {
		return fmt.Sprintf("native type=%d %q at %d:%d, ANTLR type=%d %q at %d:%d",
			n.Type, n.Text(ts.Src), n.Line, n.Column, a.GetTokenType(), a.GetText(), a.GetLine(), a.GetColumn())
	}
	return ""
}

// compareNode compares a native node with an ANTLR context, recursively.
func compareNode(file string, ts *nativegala.GalaTokens, n nativegala.GalaNodeRef, ctx antlr.ParserRuleContext) string {
	where := func() string {
		return fmt.Sprintf("%s: node %d (%s) at %d:%d", file, n.Index(), nativegala.GalaRuleNames[n.Rule()], n.Start().Line, n.Start().Column)
	}
	if n.Rule() != ctx.GetRuleIndex() {
		return fmt.Sprintf("%s: rule %d, ANTLR %d", where(), n.Rule(), ctx.GetRuleIndex())
	}
	if got, want := contextType(n), reflect.TypeOf(ctx).Elem().Name(); got != want {
		return fmt.Sprintf("%s: alternative %s, ANTLR %s", where(), got, want)
	}
	if d := sameToken(ts, n.Start(), ctx.GetStart()); d != "" {
		return where() + ": first token: " + d
	}
	switch stop := ctx.GetStop(); {
	case stop == nil && n.StopIndex() >= 0:
		return where() + ": ANTLR has no last token"
	case stop != nil:
		if d := sameToken(ts, n.Stop(), stop); d != "" {
			return where() + ": last token: " + d
		}
	}
	if d := compareAccessors(n, ctx); d != "" {
		return where() + ": " + d
	}
	if n.ChildCount() != ctx.GetChildCount() {
		return fmt.Sprintf("%s: %d children, ANTLR %d", where(), n.ChildCount(), ctx.GetChildCount())
	}
	for i := range n.ChildCount() {
		c := n.Child(i)
		switch a := ctx.GetChild(i).(type) {
		case antlr.TerminalNode:
			if !c.IsToken() {
				return fmt.Sprintf("%s: child %d is a node, ANTLR's a token", where(), i)
			}
			if d := sameToken(ts, c.Terminal().Token(), a.GetSymbol()); d != "" {
				return fmt.Sprintf("%s: child %d: %s", where(), i, d)
			}
		case antlr.ParserRuleContext:
			if c.IsToken() {
				return fmt.Sprintf("%s: child %d is a token, ANTLR's a node", where(), i)
			}
			if p := c.Node().Parent(); p.Index() != n.Index() {
				return fmt.Sprintf("%s: child %d's parent is node %d", where(), i, p.Index())
			}
			if d := compareNode(file, ts, c.Node(), a); d != "" {
				return d
			}
		}
	}
	return ""
}

// contextType is the name of the ANTLR context type a native node stands for:
// the labeled alternative's, or else the rule's.
func contextType(n nativegala.GalaNodeRef) string {
	name := nativegala.GalaRuleNames[n.Rule()]
	if n.Alt() > 0 {
		name = nativegala.GalaAltNames[n.Rule()][n.Alt()]
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r) + "Context"
}

// compareAccessors checks the typed getters of a node against ANTLR's on the
// rules that exercise each kind: list getters (sourceFile), rule labels
// (caseClause), labeled alternatives (pattern) and token getters (identifier).
func compareAccessors(n nativegala.GalaNodeRef, ctx antlr.ParserRuleContext) string {
	switch a := ctx.(type) {
	case *antlrgala.SourceFileContext:
		if len(nativegala.SourceFileNode{GalaNodeRef: n}.AllTopLevelDeclaration()) != len(a.AllTopLevelDeclaration()) {
			return "AllTopLevelDeclaration differs"
		}
		if n.GetText() != a.GetText() {
			return "GetText differs"
		}
	case *antlrgala.CaseClauseContext:
		cc := nativegala.CaseClauseNode{GalaNodeRef: n}
		if cc.GetGuard().Ok() != (a.GetGuard() != nil) || cc.GetBodyStmt().Ok() != (a.GetBodyStmt() != nil) ||
			cc.GetBodyBlock().Ok() != (a.GetBodyBlock() != nil) || cc.Pattern().GetText() != a.Pattern().GetText() {
			return "caseClause getters differ"
		}
		if cc.GetGuard().Ok() && cc.GetGuard().GetText() != a.GetGuard().GetText() {
			return "GetGuard differs"
		}
	case antlrgala.IPatternContext:
		p := nativegala.PatternNode{GalaNodeRef: n}
		_, rest := p.AsRestPattern()
		_, expr := p.AsExpressionPattern()
		typed, isTyped := p.AsTypedPattern()
		_, aRest := a.(*antlrgala.RestPatternContext)
		_, aExpr := a.(*antlrgala.ExpressionPatternContext)
		aTyped, aIsTyped := a.(*antlrgala.TypedPatternContext)
		if rest != aRest || expr != aExpr || isTyped != aIsTyped {
			return "labeled alternative differs"
		}
		if isTyped && typed.Type_().GetText() != aTyped.Type_().GetText() {
			return "TypedPattern.Type_ differs"
		}
	case *antlrgala.IdentifierContext:
		id := nativegala.IdentifierNode{GalaNodeRef: n}.IDENTIFIER()
		if t := a.IDENTIFIER(); id.Ok() != (t != nil) || t != nil && id.GetText() != t.GetText() {
			return "IDENTIFIER differs"
		}
	}
	return ""
}

// firstDifference shows where two renderings part ways.
func firstDifference(want, got string) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	lo := max(0, i-120)
	return fmt.Sprintf("  ANTLR: %s\n  galop: %s", want[lo:min(len(want), i+80)], got[lo:min(len(got), i+80)])
}

// TestNativeMutations runs the native flavor and ANTLR on prefixes of corpus
// files, cut at a few points: they must agree on whether the input parses
// (lexical errors included), and on the tree when it does.
func TestNativeMutations(t *testing.T) {
	fs := files(t)
	checked, differ := 0, 0
	for i := 0; i < len(fs); i += 7 {
		r := []rune(gala.StripBOM(fs[i].Src))
		for _, k := range []int{5, 11} {
			src := string(r[:len(r)*k/17])
			checked++
			want, wantOK := antlrVerdict(src)
			root, err := nativegala.Parse(src)
			switch {
			case wantOK != (err == nil):
				differ++
				t.Errorf("%s cut at %d/17: ANTLR accepts=%v, native error=%v", fs[i].Name, k, wantOK, err)
			case wantOK && root.ToStringTree() != want:
				differ++
				t.Errorf("%s cut at %d/17: trees differ\n%s", fs[i].Name, k, firstDifference(want, root.ToStringTree()))
			}
		}
	}
	t.Logf("%d/%d prefixes agree", checked-differ, checked)
}

// antlrVerdict is the GALA compiler's verdict on src with ANTLR: the tree when
// SLL, or else LL, reports no lexical or syntax error.
func antlrVerdict(src string) (string, bool) {
	for _, mode := range []int{gala.SLL, gala.LL} {
		if tree, errs := gala.ParseANTLR(src, mode); len(errs) == 0 {
			return gala.TreeString(tree), true
		}
	}
	return "", false
}

// BenchmarkParseNative lexes and parses the whole corpus with galop's native
// flavor once per iteration, the line-break rule included.
func BenchmarkParseNative(b *testing.B) {
	fs := files(b)
	b.SetBytes(int64(corpus.Bytes(fs)))
	for _, f := range fs { // untimed: builds the cached SLL DFA states
		nativegala.Parse(f.Src)
	}
	for b.Loop() {
		for _, f := range fs {
			nativegala.Parse(f.Src)
		}
	}
}

// BenchmarkParseNativeParallel is BenchmarkParseNative with files spread over
// GOMAXPROCS goroutines.
func BenchmarkParseNativeParallel(b *testing.B) {
	fs := files(b)
	b.SetBytes(int64(corpus.Bytes(fs)))
	parallel(fs, func(f corpus.File) { nativegala.Parse(f.Src) })
	for b.Loop() {
		parallel(fs, func(f corpus.File) { nativegala.Parse(f.Src) })
	}
}
