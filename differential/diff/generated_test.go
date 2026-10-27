package diff

import (
	"testing"
	"time"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
)

// TestGeneratedMatchesANTLR runs galop's two flavors and ANTLR on every input
// file, meant for sentences galop-sample generated from gala.g4 (run with
// -samples): random derivations reach shapes no corpus has, such as deep
// nesting, long operator chains and every alternative of every rule. ANTLR
// accepts some and rejects others (GALA's line-break rule and the grammar's
// keywords used as names can make a derivation invalid). On every input both
// flavors must agree with ANTLR on whether it parses; on accepted input they
// must build ANTLR's tree, and on rejected input report ANTLR's first syntax
// error, as the GALA compiler gets it. galop-sample writes only token texts
// the lexer reads back, so a lexical error is a generator bug and fails the
// test (the input is not compared further: lexdiff checks the lexers).
func TestGeneratedMatchesANTLR(t *testing.T) {
	flavors := []struct {
		name  string
		parse func(src string) (tree string, err error)
	}{
		{"native", func(src string) (string, error) {
			tree, _, err := nativeResult(nativegala.Parse(src))
			return tree, err
		}},
		{"antlr flavor", func(src string) (string, error) {
			ctx, err := antlrgala.NewgalaGalopParser(gala.GalopTokens(src)).SourceFile()
			if err != nil {
				return "", err
			}
			return gala.TreeString(ctx), nil
		}},
	}
	accepted, rejected, differ := 0, 0, 0
	for _, f := range files(t) {
		src := gala.StripBOM(f.Src)
		if errs := nativegala.Tokens(src).Errors; len(errs) > 0 {
			t.Errorf("%s: lexical error %v: galop-sample wrote text the lexer reads differently\n%s", f.Name, &errs[0], src)
			continue
		}
		start := time.Now()
		want, wantOK := antlrVerdict(src)
		if d := time.Since(start); d > time.Second {
			t.Logf("%s: ANTLR took %v", f.Name, d.Round(time.Millisecond))
		}
		wantErr := ""
		if wantOK {
			accepted++
		} else {
			rejected++
			wantErr, _ = gala.FirstSyntaxError(src)
		}
		for _, fl := range flavors {
			tree, err := fl.parse(src)
			switch {
			case wantOK != (err == nil):
				differ++
				t.Errorf("%s: ANTLR accepts=%v, %s error=%v\n%s", f.Name, wantOK, fl.name, err, src)
			case wantOK && tree != want:
				differ++
				t.Errorf("%s: %s tree differs\n%s", f.Name, fl.name, firstDifference(want, tree))
			case !wantOK && compilerStyle(err) != wantErr:
				differ++
				t.Errorf("%s: %s first error differs:\n ANTLR: %s\n galop: %s\n%s", f.Name, fl.name, wantErr, compilerStyle(err), src)
			}
		}
	}
	t.Logf("%d inputs: ANTLR accepts %d, rejects %d; %d differences", accepted+rejected, accepted, rejected, differ)
	if accepted == 0 {
		t.Error("ANTLR accepted no input: the generator no longer produces valid code")
	}
}

// TestParseWithStatsMatchesParse checks that the harness's counting entry
// points parse as the generated ones do: the same verdict and tree, so the
// performance gate counts the parse galop's users get.
func TestParseWithStatsMatchesParse(t *testing.T) {
	for _, f := range files(t) {
		src := gala.StripBOM(f.Src)
		want, _, wantErr := nativeResult(nativegala.Parse(src))
		root, _, err := nativegala.ParseWithStats(src)
		got, _, gotErr := nativeResult(root, err)
		if got != want || (wantErr == nil) != (gotErr == nil) {
			t.Errorf("%s: native ParseWithStats differs from Parse", f.Name)
		}
		ref, refErr := antlrgala.NewgalaGalopParser(gala.GalopTokens(src)).SourceFile()
		ctx, _, err := antlrgala.ParseGalopWithStats(gala.GalopTokens(src))
		if (refErr == nil) != (err == nil) || refErr == nil && gala.TreeString(ref) != gala.TreeString(ctx) {
			t.Errorf("%s: ParseGalopWithStats differs from SourceFile", f.Name)
		}
	}
}
