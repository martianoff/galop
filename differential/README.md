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
| `antlrgala/` | ANTLR's generated parser (`-package antlrgala`), plus `Companions`, the hook for parsers that build the same tree types |
| `gala/` | `Tokens`, `ParseANTLR`, the `Parser` type and the parser registry |
| `corpus/` | Loading input: the Bazel corpus manifest, a directory, or files |
| `diff/` | Parallel tree comparison; the corpus tests and `testing.B` benchmarks |
| `lexdiff/` | galop's generated lexer against ANTLR's, token by token over the corpus; lexer benchmarks |
| `cmd/treedump` | Print each file's tree (ANTLR's `ToStringTree`) |
| `cmd/difftest` | Compare two parsers over a corpus; exit 1 on any mismatch |
| `cmd/bench` | Lexer and parser timings, warm and in a fresh process |

## The corpus

Every `.gala` file of [martianoff/gala](https://github.com/martianoff/gala) at release
0.92.1, the release `gala.g4` was copied from: 723 files, 1.8 MB. Bazel fetches the
release archive (pinned by sha256 in `MODULE.bazel`); `//differential:corpus` lists it.

One file, `examples/slice_type_param.gala`, is not valid under `gala.g4` (GALA's own
tests skip it too). The tests list it explicitly.

## Running

```sh
bazel test //differential/...                       # the corpus tests
bazel run //differential/cmd/difftest               # antlr-lenient vs antlr-ll over the corpus
bazel run //differential/cmd/difftest -- -a antlr-sll -b antlr-ll
bazel run //differential/cmd/treedump -- -parser antlr-ll path/to/file.gala
bazel run -c opt //differential/cmd/bench           # timing table (Markdown)
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
  `//tools/lexgen`) gives ANTLR's tokens, errors and stream positions on every corpus
  file, on mutated copies (truncated, with stray characters) and on nested
  interpolations past its static DFA depth.
- `antlr-lenient` and `antlr-ll` build the same tree for all 723 files, error-recovered
  trees included, so either one is the reference.
- Where SLL alone builds a tree without a syntax error, it is LL's tree. SLL rejects 15
  files that LL accepts; those are the decisions that need full-context prediction, and
  the test logs them.
- LL accepts every corpus file but the one listed above.

## Plugging in a parser

A parser is a `gala.Parser`:

```go
type Parser func(stream antlr.TokenStream) (antlr.ParserRuleContext, error)
```

It gets the token stream from `gala.Tokens` and returns the `sourceFile` tree, plus an
error when there are syntax errors. Trees are compared by ANTLR's `ToStringTree`, so a
parser may build its own tree types, but galop's companion flavor builds ANTLR's
generated context types. It lives in the `antlrgala` package next to ANTLR's output.
To add one:

1. Add its file to `antlrgala/` and to the `srcs` of `//differential/antlrgala`.
2. Register it from that file:

   ```go
   func init() { Companions["galop"] = ParseGalop }
   ```

Then `bazel run //differential/cmd/difftest -- -a galop -b antlr-lenient` compares it with
ANTLR over the corpus.
