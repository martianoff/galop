package diff

import (
	"flag"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/mutate"
	"github.com/martianoff/galop/differential/nativegala"
)

var (
	recoveryMutants = flag.Int("recovery_mutants", 300, "TestRecoveryMatchesANTLR: how many rejected mutants to check")
	recoveryEdits   = flag.Int("recovery_edits", 300, "TestRecoveryMatchesANTLR: how many rejected editor-style edits to check")
)

// errorContext is what the GALA compiler's error hints read of an error:
// the node of the rule the parser was in (-1 when ANTLR's context for it
// is not in the tree: choosing a labeled alternative replaced it), the
// node's number of children then, and the offending token's index.
type errorContext struct {
	node        int32
	kids, token int
}

// recoveredByANTLR is what ANTLR gives for src, a file with a syntax error,
// as the GALA compiler parses it: every syntax error with its context, and
// the error-recovered tree converted to the native tree.
func recoveredByANTLR(src string) ([]string, []errorContext, *nativegala.GalaTree) {
	ctx, stream, errs := gala.ParseRecovered(src)
	tree, _, ctxs := antlrgala.GalaToNativeContexts(ctx, stream, src)
	node := make(map[antlr.ParserRuleContext]int32, len(ctxs))
	for i, c := range ctxs {
		node[c] = int32(i)
	}
	// The native index of each stream token: the default-channel ones before it.
	native := make([]int, 0, stream.Size())
	n := 0
	for _, t := range stream.GetAllTokens() {
		native = append(native, n)
		if t.GetChannel() == antlr.TokenDefaultChannel {
			n++
		}
	}
	texts := make([]string, len(errs))
	contexts := make([]errorContext, len(errs))
	for i, e := range errs {
		texts[i] = e.Text
		id, ok := node[e.Ctx]
		if !ok {
			id = -1
		}
		contexts[i] = errorContext{id, e.Kids, native[e.Token.GetTokenIndex()]}
	}
	return texts, contexts, tree
}

// recoveredByGalop is ParseSourceFileWithRecovery's errors, rendered as the
// harness renders ANTLR's, and its tree.
func recoveredByGalop(src string) ([]string, []errorContext, *nativegala.GalaTree) {
	root, errs := nativegala.ParseSourceFileWithRecovery(nativegala.Tokens(src))
	contexts := make([]errorContext, len(errs))
	for i, e := range errs {
		contexts[i] = errorContext{e.Node, e.Kids, e.Token}
	}
	return renderErrors(errs), contexts, root.Tree()
}

func renderErrors(errs []nativegala.GalaParseError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = fmt.Sprintf("%d:%d: %s", e.Line, e.Column, gala.HideNewlineTokens(e.Msg))
	}
	return out
}

// sameRecoveredTree describes the first difference between two trees,
// error tokens included, or "".
func sameRecoveredTree(want, got *nativegala.GalaTree) string {
	switch {
	case got == nil:
		return "no tree"
	case !reflect.DeepEqual(want.Tokens, got.Tokens):
		return "tokens differ"
	case !reflect.DeepEqual(want.Hidden, got.Hidden):
		return "hidden tokens differ"
	case len(want.Nodes) != len(got.Nodes):
		return fmt.Sprintf("%d nodes, want %d", len(got.Nodes), len(want.Nodes))
	}
	for i := range want.Nodes {
		if want.Nodes[i] != got.Nodes[i] {
			return fmt.Sprintf("node %d (%s) is %+v, want %+v", i, nativegala.GalaRuleNames[want.Nodes[i].Rule], got.Nodes[i], want.Nodes[i])
		}
	}
	switch {
	case !slices.Equal(want.Kids, got.Kids):
		return "child references differ"
	case !slices.Equal(want.ErrorKids, got.ErrorKids):
		return fmt.Sprintf("error tokens differ: %v, want %v", got.ErrorKids, want.ErrorKids)
	case !slices.Equal(want.Conjured, got.Conjured):
		return fmt.Sprintf("conjured tokens differ: %+v, want %+v", got.Conjured, want.Conjured)
	case !slices.Equal(want.ConjuredText, got.ConjuredText):
		return fmt.Sprintf("conjured texts differ: %q, want %q", got.ConjuredText, want.ConjuredText)
	case !slices.Equal(want.TokenLabels, got.TokenLabels):
		return "token labels differ"
	case want.Next != got.Next:
		return fmt.Sprintf("next token %d, want %d", got.Next, want.Next)
	}
	return ""
}

// recoveryOutcome is how galop's recovery compares with ANTLR's on one input.
type recoveryOutcome struct {
	firstError, errors, tree bool
	// contexts says every error has ANTLR's context (see errorContext);
	// replaced counts errors whose ANTLR context is not in the tree.
	contexts bool
	replaced int
	diff     string
	// ANTLR's errors, error tokens and conjured tokens.
	nErrors, nErrorKids, nConjured int
}

func compareRecovery(src string) recoveryOutcome {
	wantErrs, wantCtx, wantTree := recoveredByANTLR(src)
	gotErrs, gotCtx, gotTree := recoveredByGalop(src)
	o := recoveryOutcome{
		firstError: len(wantErrs) > 0 && len(gotErrs) > 0 && wantErrs[0] == gotErrs[0],
		errors:     slices.Equal(wantErrs, gotErrs),
		nErrors:    len(wantErrs),
		nErrorKids: len(wantTree.ErrorKids),
		nConjured:  len(wantTree.Conjured),
		contexts:   true,
	}
	d := sameRecoveredTree(wantTree, gotTree)
	o.tree = d == ""
	if !o.errors {
		o.diff = "errors:\n ANTLR: " + strings.Join(wantErrs, "\n        ") + "\n galop: " + strings.Join(gotErrs, "\n        ")
	}
	if !o.tree {
		o.diff += "\n tree: " + d
	}
	// Where the errors agree, so must what the hints read.
	if o.errors {
		for i, w := range wantCtx {
			g := gotCtx[i]
			if w.node < 0 {
				o.replaced++
				g.node = -1
			}
			if g != w {
				o.contexts = false
				o.diff += fmt.Sprintf("\n error %d: node, children, token %+v, want %+v", i, g, w)
			}
		}
	}
	return o
}

// recoveryStats counts outcomes for one kind of input.
type recoveryStats struct {
	name                          string
	inputs, first, errors, trees  int
	multi, nErrors, nErrorKids    int
	nConjured, contexts, replaced int
}

func (s *recoveryStats) add(o recoveryOutcome) {
	s.inputs++
	s.nErrors += o.nErrors
	s.nErrorKids += o.nErrorKids
	s.nConjured += o.nConjured
	s.replaced += o.replaced
	if o.contexts {
		s.contexts++
	}
	if o.nErrors > 1 {
		s.multi++
	}
	if o.firstError {
		s.first++
	}
	if o.errors {
		s.errors++
	}
	if o.tree {
		s.trees++
	}
}

func (s recoveryStats) String() string {
	pct := func(n int) string {
		return strconv.FormatFloat(100*float64(n)/float64(max(s.inputs, 1)), 'f', 1, 64) + "%"
	}
	return fmt.Sprintf("%s: %d inputs (%d with several errors; ANTLR reports %d errors, %d error tokens, %d conjured), first error %s, all errors %s, tree %s, error contexts %s (%d errors in a replaced context)",
		s.name, s.inputs, s.multi, s.nErrors, s.nErrorKids, s.nConjured, pct(s.first), pct(s.errors), pct(s.trees), pct(s.contexts), s.replaced)
}

// editLike changes src the way a file being edited is broken: cut off at a
// token, an opening bracket left unclosed, a closing one removed, or the
// rest of a line removed after a token, which leaves a half-typed
// expression or statement.
func editLike(r *rand.Rand, src string) string {
	toks := nativegala.Tokens(src).Tokens
	if len(toks) < 2 {
		return src
	}
	at := toks[r.Intn(len(toks)-1)]
	switch r.Intn(4) {
	case 0:
		return src[:at.End]
	case 1:
		open := []string{"(", "[", "{"}[r.Intn(3)]
		return src[:at.End] + open + src[at.End:]
	case 2:
		var closers []nativegala.GalaToken
		for _, t := range toks {
			if s := src[t.Start:t.End]; s == ")" || s == "]" || s == "}" {
				closers = append(closers, t)
			}
		}
		if len(closers) == 0 {
			return src[:at.End]
		}
		c := closers[r.Intn(len(closers))]
		return src[:c.Start] + src[c.End:]
	}
	end := strings.IndexByte(src[at.End:], '\n')
	if end < 0 {
		return src[:at.End]
	}
	return src[:at.End] + src[int(at.End)+end:]
}

// TestRecoveryMatchesANTLR compares ParseSourceFileWithRecovery with ANTLR's
// parse of files with syntax errors, as the GALA compiler parses them (LL
// prediction, the default error strategy, a fresh process): every error,
// in order, and the error-recovered tree, converted by galop's converter.
// The inputs are rejected mutants (see mutate.Mutate) and editor-style
// edits of the corpus (see editLike).
func TestRecoveryMatchesANTLR(t *testing.T) {
	srcs := mutationSources(t)
	for _, kind := range []struct {
		name   string
		n      int
		change func(*rand.Rand, string) string
	}{
		{"mutants", *recoveryMutants, mutate.Mutate},
		{"edits", *recoveryEdits, editLike},
	} {
		stats := recoveryStats{name: kind.name}
		shown := 0
		for _, src := range rejectedCopies(rand.New(rand.NewSource(*mutantSeed)), srcs, kind.change, kind.n) {
			o := compareRecovery(src)
			stats.add(o)
			if !o.errors || !o.tree || !o.contexts {
				if shown < 20 {
					t.Errorf("%s %d:\n%s", kind.name, stats.inputs, o.diff)
				}
				shown++
			}
		}
		if stats.inputs == 0 {
			t.Fatalf("%s: no rejected input was compared", kind.name)
		}
		t.Log(stats)
	}
}

// TestRecoveryWalkOnValidInput runs the recovering walk on every corpus file
// native parsing accepts: without errors it must build native parsing's
// tree and report nothing.
func TestRecoveryWalkOnValidInput(t *testing.T) {
	checked := 0
	for _, f := range files(t) {
		ts := nativegala.Tokens(f.Src)
		root, err := nativegala.ParseSourceFile(ts)
		if err != nil {
			continue
		}
		checked++
		tree, _, errs := nativegala.RecoverAlways(nativegala.Tokens(f.Src))
		if len(errs) > 0 {
			t.Errorf("%s: the recovering walk reports %v", f.Name, renderErrors(errs))
			continue
		}
		if d := sameRecoveredTree(root.Tree(), tree); d != "" {
			t.Errorf("%s: the recovering walk's tree: %s", f.Name, d)
		}
	}
	t.Logf("%d files", checked)
}

// rejectedInputs are n mutants and n editor-style edits of the corpus that
// lex and that native parsing rejects.
func rejectedInputs(tb testing.TB, n int) []string {
	srcs := mutationSources(tb)
	r := rand.New(rand.NewSource(*mutantSeed))
	return append(rejectedCopies(r, srcs, mutate.Mutate, n), rejectedCopies(r, srcs, editLike, n)...)
}

// rejectedCopies makes up to n changed copies of srcs that lex and that native
// parsing rejects.
func rejectedCopies(r *rand.Rand, srcs []string, change func(*rand.Rand, string) string, n int) []string {
	var out []string
	for tries := 0; len(out) < n && tries < 50*n; tries++ {
		src := change(r, srcs[r.Intn(len(srcs))])
		ts := nativegala.Tokens(src)
		if len(ts.Errors) > 0 {
			continue
		}
		if _, err := nativegala.ParseSourceFile(ts); err != nil {
			out = append(out, src)
		}
	}
	return out
}

// BenchmarkRecovery times a recovering parse of files with syntax errors,
// lexing included: galop's, and ANTLR's LL parse with its default error
// strategy over a DFA that stays warm between parses, as in a long-running
// editor process.
//
//	bazel run -c opt //differential/diff:diff_test -- -test.run='^$' -test.bench=Recovery -test.benchmem
func BenchmarkRecovery(b *testing.B) {
	srcs := rejectedInputs(b, 200)
	b.Run("galop", func(b *testing.B) {
		for range b.N {
			for _, src := range srcs {
				nativegala.ParseSourceFileWithRecovery(nativegala.Tokens(src))
			}
		}
	})
	b.Run("antlr", func(b *testing.B) {
		parse := gala.ANTLRParser(gala.LL)
		for range b.N {
			for _, src := range srcs {
				parse(gala.Tokens(src))
			}
		}
	})
}
