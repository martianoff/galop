package convert

import (
	"testing"

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
