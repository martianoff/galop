package convert

import (
	"strings"
	"testing"
	"time"

	"github.com/antlr4-go/antlr/v4"

	la "github.com/martianoff/galop/differential/convert/labels/antlr"
	ln "github.com/martianoff/galop/differential/convert/labels/native"
)

// TestLabelGettersMatchANTLR checks the getters of labels that match more
// than once: as in ANTLR, a non-list label's getter gives its last match.
func TestLabelGettersMatchANTLR(t *testing.T) {
	src := "pair c , d ; seq x y z ; nest ( p , q ) ; r ; end opt k 7 ; mix a u b v ; both m n ;"
	want := []string{"d", "z", "r", "7", "v", "n"}
	ctx, _, err := parseANTLR(src)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ln.ParseProg(ln.LabelsLex(src))
	if err != nil {
		t.Fatal(err)
	}
	items, nitems := ctx.(*la.ProgContext).AllItem(), root.AllItem()
	if len(items) != len(want) || len(nitems) != len(want) {
		t.Fatalf("%d items, native %d, want %d", len(items), len(nitems), len(want))
	}
	for i, it := range items {
		var got, ngot string
		switch c := it.(type) {
		case *la.PairContext:
			n, _ := nitems[i].AsPair()
			got, ngot = c.GetA().GetText(), n.GetA().GetText()
		case *la.SeqContext:
			n, _ := nitems[i].AsSeq()
			got, ngot = c.GetX().GetText(), n.GetX().GetText()
		case *la.NestContext:
			n, _ := nitems[i].AsNest()
			got, ngot = c.GetN().GetText(), n.GetN().GetText()
			// The first nested node's v matched p, then q.
			first := c.AllNode()[0].(*la.NodeContext)
			if v, nv := first.GetV().GetText(), n.AllNode()[0].GetV().GetText(); v != "q" || nv != v {
				t.Errorf("nest: GetV is %q, native %q, want q", v, nv)
			}
		case *la.OptContext:
			n, _ := nitems[i].AsOpt()
			got, ngot = c.GetO().GetText(), n.GetO().GetText()
		case *la.MixContext:
			n, _ := nitems[i].AsMix()
			got, ngot = c.GetM().GetText(), n.GetM().GetText()
		case *la.BothContext:
			n, _ := nitems[i].AsBoth()
			got, ngot = c.GetB().GetText(), n.GetB().GetText()
		}
		if got != want[i] || ngot != got {
			t.Errorf("item %d: ANTLR's getter gives %q, native %q, want %q", i, got, ngot, want[i])
		}
	}
}

// TestAlignmentIsPolynomial converts a node the aligner cannot match: an
// Amb node of 40 IDs and no ';', built by hand. Every way to split the IDs
// between the loop's alternatives fails; without remembering failures the
// search would try about 2^40 of them.
func TestAlignmentIsPolynomial(t *testing.T) {
	src := "amb" + strings.Repeat(" x", 40)
	stream := antlr.NewCommonTokenStream(la.NewLabelsLexer(antlr.NewInputStream(src)), antlr.TokenDefaultChannel)
	stream.Fill()
	ctx := la.NewAmbContext(nil, la.NewItemContext(nil, nil, -1))
	toks := stream.GetAllTokens()
	for _, tok := range toks[:len(toks)-1] {
		ctx.AddTokenNode(tok)
	}
	ctx.SetStart(toks[0])
	ctx.SetStop(toks[len(toks)-2])
	done := make(chan *ln.LabelsTree)
	go func() {
		tree, _ := la.LabelsToNative(ctx, stream, src)
		done <- tree
	}()
	select {
	case tree := <-done:
		if n := len(tree.TokenLabels); n != 0 {
			t.Errorf("%d token labels from a node that does not align, want ANTLR's field (none)", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the alignment did not finish in 10 s")
	}
}
