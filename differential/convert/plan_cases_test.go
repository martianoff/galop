package convert

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	gn "github.com/martianoff/galop/differential/convert/plan/native"
)

// TestPrecedenceOutsideTheDecisionsInvocation checks errors whose position
// depends on prediction leaving the decision's rule invocation: inside
// `a LIKE ISO u"" < c`, the operand of LIKE is parsed at a precedence that
// rules out `<=`, but past the operand's end `<=` continues the outer
// expression, where that precedence no longer applies. ANTLR's prediction
// reads on to the next token before it fails; so must galop's.
func TestPrecedenceOutsideTheDecisionsInvocation(t *testing.T) {
	for _, src := range []string{
		`a LIKE ISO u"" < c <=  `,
		`  -    iso ''  MATCH_MOST  >> `,
	} {
		want, _, _, _ := recoverANTLR(src)
		got, _, _ := recoverGalop(src)
		if !slices.Equal(want, got) {
			t.Errorf("%q: errors %q, ANTLR's %q", src, got, want)
		}
		_, _, aerr := parseANTLR(src)
		_, nerr := parseNative(src)
		var se *gn.PlanSyntaxError
		if aerr == nil || !errors.As(nerr, &se) || fmt.Sprintf("%d:%d: %s", se.Line, se.Column, se.Msg) != aerr.Error() {
			t.Errorf("%q: first error %v, ANTLR's %v", src, nerr, aerr)
		}
	}
}
