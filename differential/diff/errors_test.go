package diff

import (
	"flag"
	"math/rand"
	"strings"
	"testing"

	"github.com/martianoff/galop/differential/antlrgala"
	"github.com/martianoff/galop/differential/gala"
	"github.com/martianoff/galop/differential/nativegala"
)

var errorMutants = flag.Int("error_mutants", 200, "TestSyntaxErrors: how many rejected mutants to check")

// flavors are galop's parsers, each returning its first syntax error for src
// as compilerStyle renders it.
var flavors = []struct {
	name  string
	parse func(src string) string
}{
	{"native", func(src string) string {
		_, err := nativegala.ParseSourceFile(nativegala.Tokens(src))
		return compilerStyle(err)
	}},
	{"antlr flavor", func(src string) string {
		_, err := antlrgala.NewgalaGalopParser(gala.GalopTokens(src)).SourceFile()
		return compilerStyle(err)
	}},
}

// TestSyntaxErrors compares the first syntax error of galop's parsers with
// ANTLR's on mutated corpus files that ANTLR rejects: the same position and
// the same message, as the GALA compiler reports them. Files with lexical
// errors are skipped; both lexers report those the same way.
func TestSyntaxErrors(t *testing.T) {
	var srcs []string
	for _, f := range files(t) {
		if len(f.Src) < 20000 {
			srcs = append(srcs, gala.StripBOM(f.Src))
		}
	}
	r := rand.New(rand.NewSource(*mutantSeed))
	checked := 0
	same := make([]int, len(flavors))
	samePos := make([]int, len(flavors))
	for checked < *errorMutants {
		src := mutate(r, srcs[r.Intn(len(srcs))])
		if len(nativegala.Tokens(src).Errors) > 0 {
			continue
		}
		want, rejected := gala.FirstSyntaxError(src)
		if !rejected {
			continue
		}
		checked++
		for i, f := range flavors {
			got := f.parse(src)
			if got == want {
				same[i]++
				samePos[i]++
				continue
			}
			if errorPosition(got) == errorPosition(want) {
				samePos[i]++
			}
			t.Errorf("mutant %d, %s:\n ANTLR: %s\n galop: %s", checked, f.name, want, got)
		}
	}
	for i, f := range flavors {
		t.Logf("%s: %d rejected mutants, same position %d (%.0f%%), same error %d (%.0f%%)", f.name, checked,
			samePos[i], 100*float64(samePos[i])/float64(checked), same[i], 100*float64(same[i])/float64(checked))
	}
}

// TestSyntaxErrorCases compares the first syntax error of galop's parsers
// with ANTLR's on inputs where they once differed.
func TestSyntaxErrorCases(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			// The prediction in pattern at `if` fails at EOF. After the second
			// `^` ANTLR reuses the DFA state built after the first, with its
			// flags, so it finds no alternative to fall back on.
			"reused DFA state keeps its flags",
			`package _n val _n map [ default ] default = ( func ( ) , Z map [ Z ] Z = { case if ( D_v ) "!\+" else { } ^ ( ) => { } >> { case default => map [ _M6 ] map [ Z ] default { } } ^ ( ) match`,
		},
	} {
		want, rejected := gala.FirstSyntaxError(tc.src)
		if !rejected {
			t.Fatalf("%s: ANTLR accepts the input", tc.name)
		}
		for _, f := range flavors {
			if got := f.parse(tc.src); got != want {
				t.Errorf("%s, %s:\n ANTLR: %s\n galop: %s", tc.name, f.name, want, got)
			}
		}
	}
}

// errorPosition is the "line:col" of an error rendered as "line:col: message".
func errorPosition(err string) string {
	pos, _, _ := strings.Cut(err, ": ")
	return pos
}

// compilerStyle renders a galop syntax error, "line L:C message", as the
// harness renders ANTLR's: "L:C: message", with the re-typed tokens hidden.
func compilerStyle(err error) string {
	if err == nil {
		return "no error"
	}
	rest, ok := strings.CutPrefix(err.Error(), "line ")
	if !ok {
		return err.Error()
	}
	pos, msg, _ := strings.Cut(rest, " ")
	return pos + ": " + gala.HideNewlineTokens(msg)
}
