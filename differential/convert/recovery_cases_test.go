package convert

import (
	"slices"
	"strings"
	"testing"

	gn "github.com/martianoff/galop/differential/convert/recovery/native"
)

// TestRecoveryCases checks inputs where ANTLR's recovery depends on how its
// generated code decides and matches. Each states ANTLR's errors (checked
// against ANTLR too), so a change on either side shows.
func TestRecoveryCases(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// (A | B)* is an LL(1) loop in ANTLR's generated code: d is in
			// neither alternative, so the loop exits and C's match fails.
			"loop with a block body exits on one token",
			"z d ;",
			"1:2: mismatched input 'd' expecting {'a', 'b', 'c'}",
		},
		{
			// The set match fails without reporting; C's match then inserts
			// 'c' and reports it, but Match returns nil while the set's error
			// is pending, so no conjured token joins the tree.
			"token match after a failed set match",
			"l d ;",
			"1:2: missing 'c' at 'd'",
		},
	} {
		wantErrs, wantTree, crashed := recoverANTLR(tc.src)
		if crashed {
			t.Fatalf("%s: ANTLR crashed", tc.name)
		}
		if got := strings.Join(wantErrs, "\n"); got != tc.want {
			t.Errorf("%s: ANTLR reports\n%s\nwant\n%s", tc.name, got, tc.want)
		}
		gotErrs, gotTree := recoverGalop(tc.src)
		if !slices.Equal(gotErrs, wantErrs) {
			t.Errorf("%s: galop reports %q, ANTLR %q", tc.name, gotErrs, wantErrs)
		}
		if d := sameRecoveredTree(wantTree, gotTree); d != "" {
			t.Errorf("%s: tree: %s", tc.name, d)
		}
		// The first-error replay of a plain parse agrees too.
		_, err := gn.ParseProg(gn.RecoveryLex(tc.src))
		if first, _, _ := strings.Cut(tc.want, "\n"); err == nil || !strings.HasSuffix(err.Error(), first[strings.Index(first, ": ")+2:]) {
			t.Errorf("%s: ParseProg reports %v, want %s", tc.name, err, first)
		}
	}
}
