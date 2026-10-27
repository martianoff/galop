// Package mutate makes the token-level mutants the differential tests and the
// performance gate feed galop and ANTLR: mostly invalid code, sometimes code
// that parses only with ANTLR's full-context prediction.
package mutate

import (
	"math/rand"
	"strings"

	"github.com/martianoff/galop/differential/nativegala"
)

// insertions are text Mutate puts into a file at a token boundary:
// stray tokens, and constructs whose decisions need full-context prediction
// (a tuple pattern or a lambda, `if x {`, newline-separated calls).
var insertions = []string{
	"(", ")", "=>", "int", "x", ".", ",", "=", "{", "}", "[", "]", ":", "...", "func ", "return\n",
	"(a, b) => ", "case (a, b) => a\n", "if x {", "x {", "\n(", "\n-", "f(a)", "Foo{a: 1}",
	// Bracket-heavy text, balanced and not, for prediction over bracketed groups.
	"()", "[]", "{}", "((", "))", "f(", "[(", "{)", "(a, (b, c)) => a", "xs[0]", "f[T](x)", "{ x }",
	"((a) => (b) => a)", "Foo{a: (1, 2)}", "\n[", "{ case (a, b) => (a) }", "m[K, V]{}", "(f(a)[0])",
	"(x int, y T[A]) => x",
}

// statements are valid statements Mutate puts on a line of its own
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

// Mutate changes src at a random token or line: drops or doubles a token,
// inserts stray text or a statement, or adds, changes or removes brackets.
func Mutate(r *rand.Rand, src string) string {
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
	if r.Intn(4) == 0 {
		return mutateBrackets(r, src, toks)
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

// mutateBrackets wraps a balanced span in a new bracket pair, or changes a
// matched pair to another kind (it stays balanced) or removes it.
func mutateBrackets(r *rand.Rand, src string, toks []nativegala.GalaToken) string {
	text := func(i int) string { return src[toks[i].Start:toks[i].End] }
	kinds := []string{"()", "[]", "{}"}
	kind := kinds[r.Intn(len(kinds))]
	var open []int
	var pairs [][2]int
	for i := range toks {
		switch text(i) {
		case "(", "[", "{":
			open = append(open, i)
		case ")", "]", "}":
			if len(open) > 0 {
				pairs = append(pairs, [2]int{open[len(open)-1], i})
				open = open[:len(open)-1]
			}
		}
	}
	if len(pairs) == 0 || r.Intn(2) == 0 {
		// Wrap the span from a token to its pair (or to itself).
		i := r.Intn(len(toks) - 1)
		j := i
		for _, p := range pairs {
			if p[0] == i {
				j = p[1]
			}
		}
		return src[:toks[i].Start] + kind[:1] + src[toks[i].Start:toks[j].End] + kind[1:] + src[toks[j].End:]
	}
	p := pairs[r.Intn(len(pairs))]
	if r.Intn(4) == 0 {
		kind = "  "
	}
	a, b := toks[p[0]], toks[p[1]]
	return src[:a.Start] + kind[:1] + src[a.End:b.Start] + kind[1:] + src[b.End:]
}
