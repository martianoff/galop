package nativegala

import (
	"flag"
	"testing"

	"github.com/martianoff/galop/differential/corpus"
)

var source corpus.Source

func init() { source.RegisterFlags(flag.CommandLine) }

// TestGuessesCoverSLL cross-checks the guessing first pass, whose scans and
// bounded automata galop plans ahead of time (the guess package), against the
// runtime predictor: at every scanned or bounded decision a first pass meets
// on the corpus, when SLL prediction over the same bracketed groups has a
// unique answer, the plan must not have ruled that alternative out. The plan
// over-approximates the parser; this keeps it from drifting away from the
// predictor.
func TestGuessesCoverSLL(t *testing.T) {
	files, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	galaGalopGuessCheck = func(p *galaGalopParser, d int, alts uint64) {
		pos, seen := p.pos, p.rp.seen
		p.first = false
		alt, ok := func() (alt int, ok bool) {
			defer func() {
				if r := recover(); r != nil {
					if _, syntax := r.(*GalaSyntaxError); !syntax {
						panic(r)
					}
					ok = false
				}
			}()
			return p.adaptivePredict(d), true
		}()
		unique := p.rp.seen == seen
		p.pos, p.rp.seen, p.first = pos, seen, true
		checked++
		if ok && unique && alts&(1<<uint(alt)) == 0 {
			t.Errorf("decision %d at line %d: SLL predicts alternative %d only, the plan leaves %b",
				d, p.ts.Tokens[pos].Line, alt, alts)
		}
	}
	defer func() { galaGalopGuessCheck = nil }()
	for _, f := range files {
		Parse(f.Src)
	}
	if checked == 0 {
		t.Fatal("no scanned or bounded decision was met")
	}
	t.Logf("%d scanned or bounded decisions checked against SLL prediction", checked)
}
