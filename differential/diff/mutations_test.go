package diff

import (
	"flag"
	"fmt"
	"math/rand"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/mutate"
	"github.com/martianoff/galop/differential/nativegala"
	"github.com/martianoff/galop/differential/nativetokens"
)

var (
	mutants    = flag.Int("mutants", 200, "TestMutations: how many mutated files to check")
	mutantSeed = flag.Int64("mutant_seed", 1, "TestMutations: the random seed")
)

// TestMutations compares galop's parsers with ANTLR on mutated corpus files:
// a token dropped or doubled, text inserted, a statement added, or a bracket
// pair wrapped around a span, changed to another kind or removed. Most
// mutants are rejected; the rest are where SLL prediction and LL prediction
// part ways, which is what the repair of a failed SLL parse has to get right.
// Both flavors must agree with ANTLR on whether the input parses and, if it
// does, on the tree. Each flavor must also agree with the same parser
// generated with -Xno-brackets on everything, the first error included:
// reading bracketed groups as one symbol changes neither trees nor errors.
// -mutants and -mutant_seed size the run.
func TestMutations(t *testing.T) {
	srcs := mutationSources(t)
	// Each flavor parses with the repair of a failed SLL parse (rerun false)
	// and with ANTLR's whole-input LL rerun (rerun true). On error, where says
	// where the parse left off: the error text, with its position, and for the
	// antlr flavor the token stream's index.
	antlrFlavor := func(src string, parse func(antlr.TokenStream) (antlr.ParserRuleContext, error)) (string, string, error) {
		stream := gala.GalopTokens(src)
		ctx, err := parse(stream)
		if err != nil {
			return "", fmt.Sprintf("%v, stream at %d", err, stream.Index()), err
		}
		return gala.TreeString(ctx), "", nil
	}
	flavors := []struct {
		name  string
		parse func(src string, rerun bool) (tree, where string, err error)
		// tokens parses with the parser generated with -Xno-brackets.
		tokens func(src string) (tree, where string, err error)
	}{
		{"native", func(src string, rerun bool) (string, string, error) {
			parse := nativegala.Parse
			if rerun {
				parse = nativegala.ParseByRerun
			}
			return nativeResult(parse(src))
		}, func(src string) (string, string, error) {
			return nativeResult(nativetokens.Parse(src))
		}},
		{"antlr flavor", func(src string, rerun bool) (string, string, error) {
			if rerun {
				return antlrFlavor(src, antlrgala.ParseGalopByRerun)
			}
			return antlrFlavor(src, func(s antlr.TokenStream) (antlr.ParserRuleContext, error) {
				return antlrgala.NewgalaGalopParser(s).SourceFile()
			})
		}, func(src string) (string, string, error) {
			return antlrFlavor(src, antlrgala.ParseGalopTokens)
		}},
	}
	r := rand.New(rand.NewSource(*mutantSeed))
	accepted, differ := 0, 0
	for i := 0; i < *mutants; i++ {
		src := mutate.Mutate(r, srcs[r.Intn(len(srcs))])
		want, wantOK := antlrVerdict(src)
		if wantOK {
			accepted++
		}
		for _, f := range flavors {
			got, where, err := f.parse(src, false)
			// The error must match here; where each parse stopped is checked
			// against the LL rerun's below.
			ref, refWhere, refErr := f.tokens(src)
			switch {
			case (err == nil) != (refErr == nil) || err != nil && err.Error() != refErr.Error():
				differ++
				t.Errorf("mutant %d: %s with brackets error=%v, without error=%v\n%s", i, f.name, err, refErr, src)
			case err == nil && got != ref:
				differ++
				t.Errorf("mutant %d: %s trees differ with and without brackets\n%s", i, f.name, firstDifference(ref, got))
			}
			switch {
			case wantOK != (err == nil):
				differ++
				t.Errorf("mutant %d: ANTLR accepts=%v, %s error=%v\n%s", i, wantOK, f.name, err, src)
			case wantOK && got != want:
				differ++
				t.Errorf("mutant %d: %s tree differs\n%s", i, f.name, firstDifference(want, got))
			case !wantOK:
				// Rejected: the error and where the parse stopped must be the
				// rerun's, not just the verdict, with brackets and without.
				_, rerunWhere, _ := f.parse(src, true)
				if where != rerunWhere {
					differ++
					t.Errorf("mutant %d: %s error differs from the LL rerun's:\n repair: %s\n rerun:  %s", i, f.name, where, rerunWhere)
				}
				if refWhere != rerunWhere {
					differ++
					t.Errorf("mutant %d: %s -Xno-brackets error differs from the LL rerun's:\n repair: %s\n rerun:  %s", i, f.name, refWhere, rerunWhere)
				}
			}
		}
	}
	t.Logf("%d mutants, %d accepted by ANTLR, %d differences", *mutants, accepted, differ)
}

// nativeResult is a native parse's tree, or its error text, for comparing.
func nativeResult[N interface{ ToStringTree() string }](root N, err error) (tree, where string, _ error) {
	if err != nil {
		return "", err.Error(), err
	}
	return root.ToStringTree(), "", nil
}

// mutationSources are the inputs the mutation tests mutate: the files under
// 20,000 bytes, BOM stripped. Large files make ANTLR's LL pass slow without
// adding much.
func mutationSources(tb testing.TB) []string {
	var srcs []string
	for _, f := range files(tb) {
		if len(f.Src) < 20000 {
			srcs = append(srcs, gala.StripBOM(f.Src))
		}
	}
	return srcs
}
