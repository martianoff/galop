package diff

import (
	"errors"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/corpus"
	"github.com/martianoff/galop/differential/gala"
)

// TestCompareWithoutTree checks that a parser returning no tree with its
// syntax error is reported, not dereferenced: galop's parser does that on
// input it rejects.
func TestCompareWithoutTree(t *testing.T) {
	f := corpus.File{Name: "x.gala", Src: "package main\n"}
	failing := func(antlr.TokenStream) (antlr.ParserRuleContext, error) {
		return nil, errors.New("1:0: syntax error")
	}
	working := gala.ANTLRParser(gala.LL)

	both := Compare(f, failing, failing)
	if !both.Same || both.ErrA == nil || both.ErrB == nil {
		t.Errorf("two parsers without a tree: got %+v, want Same with both errors", both)
	}
	one := Compare(f, working, failing)
	if one.Same || one.ErrB == nil || !strings.Contains(one.Diff, "b: (no tree)") {
		t.Errorf("one parser without a tree: got %+v, want a mismatch naming the missing tree", one)
	}
}
