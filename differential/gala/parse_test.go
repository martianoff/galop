package gala

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
)

const hello = `package main

func main() {
    Println("hi")
}
`

// retyped lists the re-typed tokens of src as "NAME@line:col".
func retyped(src string) []string {
	antlrgala.GalaParserInit()
	names := antlrgala.GalaParserStaticData.SymbolicNames
	stream := Tokens(src)
	stream.Fill()
	var out []string
	for _, tok := range stream.GetAllTokens() {
		if tt := tok.GetTokenType(); tt > 0 && tt < len(names) && strings.HasPrefix(names[tt], "NL_") {
			out = append(out, fmt.Sprintf("%s@%d:%d", names[tt], tok.GetLine(), tok.GetColumn()))
		}
	}
	return out
}

func TestTokensRetypeLineStarts(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"paren after a call on the line before", "package main\n\nfunc f() {\n    g()\n    (1, 2)\n}\n", []string{"NL_LPAREN@5:4"}},
		{"star against its operand starts a statement", "package main\n\nfunc f() {\n    val p = &n\n    *p = 5\n}\n", []string{"NL_STAR@5:4"}},
		{"amp against its operand starts a statement", "package main\n\nfunc f() {\n    g()\n    &n\n}\n", []string{"NL_AMP@5:4"}},
		{"star followed by a space continues the line", "package main\n\nfunc f() {\n    val x = a\n    * b\n}\n", nil},
		{"call on one line", "package main\n\nfunc f() {\n    g()(1, 2)\n}\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := retyped(c.src); !slices.Equal(got, c.want) {
				t.Errorf("re-typed tokens = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseANTLR(t *testing.T) {
	for _, mode := range []int{SLL, LL} {
		tree, errs := ParseANTLR(hello, mode)
		if len(errs) > 0 {
			t.Fatalf("mode %d: unexpected errors %v", mode, errs)
		}
		if _, ok := tree.(antlrgala.ISourceFileContext); !ok {
			t.Fatalf("mode %d: tree is %T, want a sourceFile", mode, tree)
		}
	}
}

func TestParseANTLRStripsBOM(t *testing.T) {
	plain, errs := ParseANTLR(hello, LL)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	withBOM, errs := ParseANTLR(bom+hello, LL)
	if len(errs) > 0 {
		t.Fatalf("BOM not stripped: %v", errs)
	}
	if TreeString(plain) != TreeString(withBOM) {
		t.Errorf("trees differ with a BOM:\n%s\n%s", TreeString(plain), TreeString(withBOM))
	}
}

func TestParseANTLRReportsErrors(t *testing.T) {
	_, errs := ParseANTLR("package main\n\nfunc main() {\n    val = 1\n}\n", LL)
	if len(errs) == 0 {
		t.Fatal("no syntax error reported")
	}
	if got := errs[0].Error(); !strings.HasPrefix(got, "4:") {
		t.Errorf("first error %q, want it on line 4", got)
	}
}

func TestParsersAgreeOnValidInput(t *testing.T) {
	want, errs := ParseANTLR(hello, LL)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for name, p := range Parsers() {
		tree, err := p(Tokens(hello))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if TreeString(tree) != TreeString(want) {
			t.Errorf("%s: tree %s, want %s", name, TreeString(tree), TreeString(want))
		}
	}
}

func TestLenientRetriesWithLL(t *testing.T) {
	src := "package main\n\nfunc main() {\n    val = 1\n}\n"
	_, llErrs := ParseANTLR(src, LL)
	_, err := Lenient(Tokens(src))
	if err == nil || len(llErrs) == 0 {
		t.Fatalf("want errors from both, got %v and %v", err, llErrs)
	}
	if err.Error() != llErrs[0].Error() && !strings.HasPrefix(err.Error(), llErrs[0].Error()) {
		t.Errorf("Lenient reported %q, want LL's %q", err, llErrs[0])
	}
}

// TestConcurrentParses parses on many goroutines at once; run it with -race.
func TestConcurrentParses(t *testing.T) {
	want, _ := ParseANTLR(hello, SLL)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				tree, errs := ParseANTLR(hello, SLL)
				if len(errs) > 0 || TreeString(tree) != TreeString(want) {
					t.Errorf("concurrent parse differs: %v", errs)
					return
				}
			}
		}()
	}
	wg.Wait()
}

var _ antlr.TokenStream = Tokens("")
