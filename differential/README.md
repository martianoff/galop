# Differential harness

A Go harness that runs ANTLR 4.13.1's own Go parser for
[`gala.g4`](../testdata/grammars/gala.g4) over the GALA compiler's sources, so a
galop-generated parser can be checked against it tree for tree and timed against it.

ANTLR runs the way the GALA compiler runs it: the same token stream (GALA's line-break
rule re-types some tokens to `NL_LPAREN`, `NL_STAR` and `NL_AMP`; see
[`gala/newline.go`](gala/newline.go), ported from GALA 0.92.1), the same SLL-then-LL
strategy, and per-parse prediction-context caches so files parse in parallel safely.

It is plain Go built with Bazel only: the reference parser is generated from `gala.g4`
at build time, never checked in. `go.mod` here exists for Bazel's `go_deps`.

## Layout

| Path | What |
|------|------|
| `antlrgala/` | ANTLR's generated parser (`-package antlrgala`), galop's `antlr` flavor next to it, and `SameTypes`, the hook for parsers that build the same tree types |
| `nativegala/` | galop's `native` flavor for `gala.g4` (`-package nativegala`, no ANTLR dependency) and GALA's line-break rule ported to its token list |
| `gala/` | `Tokens`, `ParseANTLR`, the `Parser` type and the parser registry |
| `corpus/` | Loading input: the Bazel corpus manifest, a directory, files, or `galop-sample` output |
| `diff/` | Parallel tree comparison; the corpus tests (both flavors) and `testing.B` benchmarks |
| `lexdiff/` | galop's generated lexers (both flavors) against ANTLR's, token by token over the corpus; lexer benchmarks |
| `cmd/treedump` | Print each file's tree (ANTLR's `ToStringTree`) |
| `cmd/difftest` | Compare two parsers over a corpus; exit 1 on any mismatch |
| `cmd/bench` | Lexer and parser timings, warm and in a fresh process |
| `cmd/perfgate` | The performance gate: counters and time ratios against `perf/baseline.json` (see [FUZZ_AND_PERF.md](../docs/FUZZ_AND_PERF.md)) |
| `perf/` | The gate's rules and its checked-in baseline |

## The corpus

Every `.gala` file of [martianoff/gala](https://github.com/martianoff/gala) at release
0.92.1, the release `gala.g4` was copied from: 723 files, 1.8 MB. Bazel fetches the
release archive (pinned by sha256 in `MODULE.bazel`); `//differential:corpus` lists it.

One file, `examples/slice_type_param.gala`, is not valid under `gala.g4` (GALA's own
tests skip it too). The tests list it explicitly.

The **held-out corpus** is GALA code galop was not tuned on: every `.gala` file of
[gala-team](https://github.com/martianoff/gala-team),
[gala-tui](https://github.com/martianoff/gala-tui) and
[gala-server](https://github.com/martianoff/gala-server) at pinned commits of their
default branches (247 files, 3.6 MB; `//differential:heldout`). ANTLR accepts all of
them. Its two largest files need ANTLR's LL stage, so they exercise the repair of a
failed SLL parse (see [DESIGN.md](../docs/DESIGN.md#repairing-a-failed-sll-parse)).

## Running

```sh
bazel test //differential/...                       # the corpus tests
bazel run //differential/cmd/difftest               # antlr-lenient vs antlr-ll over the corpus
bazel run //differential/cmd/difftest -- -a antlr-sll -b antlr-ll
bazel run //differential/cmd/treedump -- -parser antlr-ll path/to/file.gala
bazel run -c opt //differential/cmd/bench           # timing table (Markdown)
bazel run //differential/cmd/perfgate               # the performance gate (report only)
bazel run -c opt //differential/diff:diff_test -- -test.run='^$' -test.bench=. -test.benchmem
bazel test //differential/lexdiff:lexdiff_test         # galop's lexer vs ANTLR's, token by token
```

Every tool reads the Bazel corpus by default. `-dir <dir>` or file arguments point it at
other input instead, relative to the directory you ran `bazel run` from.

Parsers, selected by name with `-a`, `-b` and `-parser`:

| Name | What |
|------|------|
| `antlr-lenient` | The GALA compiler's strategy: SLL, then LL from the start on a syntax error |
| `antlr-sll` | SLL prediction only |
| `antlr-ll` | Full-context LL prediction only |

## What the tests check

- galop's generated lexer (`antlrgala.NewgalaGalopLexer`, built from `gala.g4` by
  the galop CLI) gives ANTLR's tokens, errors and stream positions on every corpus
  file, on mutated copies (truncated, with stray characters) and on nested
  interpolations past its static DFA depth. The native flavor's lexer
  (`nativegala.GalaLex`) gives the same tokens, every channel, and errors on the
  same inputs.
- galop's antlr flavor builds ANTLR's tree on every file ANTLR accepts and
  rejects the one it rejects (`TestGalopMatchesANTLR`).
- The native flavor, with GALA's line-break rule on its token list, matches ANTLR
  on every file (`TestNativeMatchesANTLR`): every token, on and off the default
  channel; node for node the rule, the labeled alternative, the first and last
  token and the children; and the typed accessors of a few rules. On prefixes of
  the corpus files it agrees with ANTLR on whether the input parses and on the
  tree when it does (`TestNativeMutations`).
- `antlr-lenient` and `antlr-ll` build the same tree for all 723 files, error-recovered
  trees included, so either one is the reference.
- Where SLL alone builds a tree without a syntax error, it is LL's tree. SLL rejects 15
  files that LL accepts; those are the decisions that need full-context prediction, and
  the test logs them.
- LL accepts every corpus file but the one listed above.
- `heldout_test` runs the two flavors' acceptance tests above on the held-out corpus.
- On mutated corpus files (a token dropped or doubled, stray text, or a statement
  added whose decisions need full-context prediction), both flavors agree with
  ANTLR on whether the input parses and on the tree, and on rejected input give
  the same error, at the same place, as the whole-input LL rerun
  (`TestMutations`; 200 mutants
  per corpus in `diff_test` and `heldout_test`, `-mutants` and `-mutant_seed` for
  more). `mutations_large_test` and `heldout_mutations_large_test` (tagged
  `manual`) check 20,000 and 10,000; CI runs them on pushes to `main`.
- On mutated corpus files that ANTLR rejects, both flavors report ANTLR's first
  syntax error, as the GALA compiler gets it (SLL, then LL after an error): the
  same message at the same line and column (`TestSyntaxErrors`; 200 rejected
  mutants per corpus, `-error_mutants` for more; the large runs check 5,000 and
  3,000).
- `//differential/repair:repair_test` checks both flavors against ANTLR on
  [`repair/Repair.g4`](repair/Repair.g4), a grammar whose SLL prediction fails
  where LL prediction finds the parse: through the static automaton, through
  runtime prediction past deep nesting, several times in one input, with no
  viable alternative, and for an entry rule without EOF. On rejected inputs it
  also checks ANTLR's first error.
- `generated_test` and `generated_deep_test` run `TestGeneratedMatchesANTLR`, `TestMutations` and
  `TestSyntaxErrors` on random sentences of `gala.g4` that `galop-sample` generated with a
  fixed seed (`//differential:samples`, `:samples_deep`): valid code no corpus has, and
  mutated copies of it. Both flavors must give ANTLR's verdict, ANTLR's tree on accepted
  input and ANTLR's first error on rejected input. `generated_large_1_test` to `_4_test`
  (tagged `manual`) check 4,000 sentences and 10,000 mutants in four processes; CI runs it on pushes to `main`. See
  [FUZZ_AND_PERF.md](../docs/FUZZ_AND_PERF.md).
- `//differential/errors:errors_test` checks both flavors' syntax errors against
  ANTLR's on mutations of programs in [`errors/Errors.g4`](errors/Errors.g4), a
  grammar galop decides without runtime prediction.

## Plugging in a parser

A parser is a `gala.Parser`:

```go
type Parser func(stream antlr.TokenStream) (antlr.ParserRuleContext, error)
```

It gets the token stream from `gala.Tokens` and returns the `sourceFile` tree, plus an
error when there are syntax errors. Trees are compared by ANTLR's `ToStringTree`, so a
parser may build its own tree types, but galop's antlr flavor builds ANTLR's
generated context types. It lives in the `antlrgala` package next to ANTLR's output.
To add one:

1. Add its file to `antlrgala/` and to the `srcs` of `//differential/antlrgala`.
2. Register it from that file:

   ```go
   func init() { SameTypes["galop"] = ParseGalop }
   ```

Then `bazel run //differential/cmd/difftest -- -a galop -b antlr-lenient` compares it with
ANTLR over the corpus.
