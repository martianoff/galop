package milvus

import (
	"flag"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	plan "github.com/martianoff/galop/differential/convert/plan/antlr"
	"github.com/martianoff/galop/differential/convert/plan/native"
	"github.com/martianoff/galop/differential/corpus"
)

var exprs = flag.String("exprs", "", "exprs.json, by runfiles location or path")

func load(t *testing.T) []string {
	t.Helper()
	qs, err := Load(corpus.Locate(*exprs))
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) == 0 {
		t.Fatal("no expressions")
	}
	return qs
}

// TestNativeMatchesANTLR checks that on every expression galop's native
// flavor builds ANTLR's tree, so the benchmark compares parsers that do the
// same work.
func TestNativeMatchesANTLR(t *testing.T) {
	for _, q := range load(t) {
		lexer := plan.NewPlanLexer(antlr.NewInputStream(q))
		lexer.RemoveErrorListeners()
		p := plan.NewPlanParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
		p.RemoveErrorListeners()
		want := p.Expr().ToStringTree(nil, p)
		root, err := native.ParseExpr(native.PlanLex(q))
		if err != nil {
			t.Errorf("%q: %v", q, err)
			continue
		}
		if got := root.Ref().ToStringTree(); got != want {
			t.Errorf("%q:\n galop %s\n ANTLR %s", q, got, want)
		}
	}
}

// TestEveryWayParses checks that the three parsers the benchmark times accept
// every expression.
func TestEveryWayParses(t *testing.T) {
	for _, q := range load(t) {
		for name, parse := range map[string]func(string) bool{"native": Native, "LL": LL, "driver": Driver} {
			if !parse(q) {
				t.Errorf("%s rejects %q", name, q)
			}
		}
	}
}
