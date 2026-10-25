// Package diff parses a corpus with two parsers and compares their trees.
package diff

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
)

// Result is the outcome of parsing one file with both parsers.
type Result struct {
	File corpus.File
	// ErrA and ErrB are the parsers' syntax errors, if any.
	ErrA, ErrB error
	// Same reports whether the two trees print identically with ANTLR's
	// ToStringTree.
	Same bool
	// Diff describes the first subtree where the trees differ, when they do.
	Diff string
}

// Run parses every file with a and with b, on up to GOMAXPROCS goroutines,
// and returns the results in the order of files. Each parse reads its own
// token stream.
func Run(files []corpus.File, a, b gala.Parser) []Result {
	results := make([]Result, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				results[i] = Compare(files[i], a, b)
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	return results
}

// Compare parses one file with a and with b and compares the trees.
func Compare(f corpus.File, a, b gala.Parser) Result {
	treeA, errA := a(gala.Tokens(f.Src))
	treeB, errB := b(gala.Tokens(f.Src))
	r := Result{File: f, ErrA: errA, ErrB: errB}
	// A parser may return no tree with its syntax error. Two missing trees
	// agree; a missing tree and a tree differ at the root.
	switch {
	case treeA == nil && treeB == nil:
		r.Same = true
	case treeA == nil || treeB == nil:
		r.Diff = fmt.Sprintf("at the root\n  a: %s\n  b: %s", render(treeA), render(treeB))
	default:
		r.Same = gala.TreeString(treeA) == gala.TreeString(treeB)
		if !r.Same {
			r.Diff = FirstDiff(treeA, treeB)
		}
	}
	return r
}

// render is a tree in ToStringTree form, clipped for a report, or a note that
// the parser returned none.
func render(t antlr.ParserRuleContext) string {
	if t == nil {
		return "(no tree)"
	}
	return clip(gala.TreeString(t))
}

// FirstDiff describes the first node, in document order, where a and b
// differ: the rule path down to it, its position, and both subtrees in
// ToStringTree form. Children the two nodes have in common are searched
// first, so a node is reported for a differing child count only when its
// shared children all match. It is empty when the trees are the same.
func FirstDiff(a, b antlr.Tree) string {
	names := gala.RuleNames()
	var path []string
	var walk func(a, b antlr.Tree) string
	walk = func(a, b antlr.Tree) string {
		label := antlr.TreesGetNodeText(a, names, nil)
		if label == antlr.TreesGetNodeText(b, names, nil) {
			path = append(path, label)
			na, nb := a.GetChildCount(), b.GetChildCount()
			for i := range min(na, nb) {
				if d := walk(a.GetChild(i), b.GetChild(i)); d != "" {
					return d
				}
			}
			path = path[:len(path)-1]
			if na == nb {
				return ""
			}
		}
		return fmt.Sprintf("at %s%s\n  a: %s\n  b: %s", strings.Join(path, " > "), position(a),
			clip(antlr.TreesStringTree(a, names, nil)), clip(antlr.TreesStringTree(b, names, nil)))
	}
	return walk(a, b)
}

// position is where node t starts in the source, as " (line L:C)".
func position(t antlr.Tree) string {
	var tok antlr.Token
	switch n := t.(type) {
	case antlr.ParserRuleContext:
		tok = n.GetStart()
	case antlr.TerminalNode:
		tok = n.GetSymbol()
	}
	if tok == nil {
		return ""
	}
	return fmt.Sprintf(" (line %d:%d)", tok.GetLine(), tok.GetColumn())
}

// clip shortens a subtree rendering for a report.
func clip(s string) string {
	const max = 600
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... (%d more bytes)", len(s)-max)
}

// Report writes one block per mismatch and per file with syntax errors, then
// the summary line, and returns the number of mismatches.
func Report(w io.Writer, nameA, nameB string, results []Result) int {
	mismatches, failed := 0, 0
	for _, r := range results {
		if r.ErrA != nil || r.ErrB != nil {
			failed++
			fmt.Fprintf(w, "SYNTAX ERRORS %s\n", r.File.Name)
			for _, e := range []struct {
				name string
				err  error
			}{{nameA, r.ErrA}, {nameB, r.ErrB}} {
				if e.err != nil {
					fmt.Fprintf(w, "  %s: %s\n", e.name, firstLines(e.err.Error(), 5))
				}
			}
		}
		if !r.Same {
			mismatches++
			fmt.Fprintf(w, "MISMATCH %s\n  %s\n", r.File.Name, strings.ReplaceAll(r.Diff, "\n", "\n  "))
		}
	}
	fmt.Fprintf(w, "%d/%d identical (%s vs %s); %d with syntax errors\n",
		len(results)-mismatches, len(results), nameA, nameB, failed)
	return mismatches
}

// firstLines keeps the first n lines of a multi-line error, indented to sit
// under its heading.
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("... (%d more)", len(lines)-n))
	}
	return strings.Join(lines, "\n    ")
}
