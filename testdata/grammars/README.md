# Corpus grammars

Verbatim copies of upstream grammars. `galop check --roundtrip` must pass on every file
here (CI runs it). Do not edit these files to make a check pass; update them only by
copying again from upstream, and record the new source below.

| File | Source | Revision | License |
|------|--------|----------|---------|
| `gala.g4` | [martianoff/gala](https://github.com/martianoff/gala) `internal/parser/grammar/gala.g4` | `master` at `911cc059` (GALA 0.92.1) | Apache-2.0 |
| `CEL.g4` | [cel-expr/cel-go](https://github.com/cel-expr/cel-go) `parser/gen/CEL.g4` | `master` at `0ef120e6` | Apache-2.0 |
| `HavingExpression.g4` | [SigNoz/signoz](https://github.com/SigNoz/signoz) `grammar/HavingExpression.g4` | `main` at `f7220ae7` | MIT |
| `Plan.g4` | [milvus-io/milvus](https://github.com/milvus-io/milvus) `internal/parser/planparserv2/Plan.g4` | `master` at `301f3bf5` | Apache-2.0 |
| `grulev3.g4` | [hyperjumptech/grule-rule-engine](https://github.com/hyperjumptech/grule-rule-engine) `antlr/grulev3.g4` | `master` at `f23b4a27` | Apache-2.0 |
| `fileseq.g4` | [justinfx/gofileseq](https://github.com/justinfx/gofileseq) `grammar/fileseq.g4` | `master` at `2014c3fe` | MIT |

`.gitattributes` stores every grammar with LF line endings; `grulev3.g4` has CRLF
upstream.

`CEL.g4`, `HavingExpression.g4`, `Plan.g4` and `grulev3.g4` have left-recursive rules.
`//differential/convert` builds both flavors of each and checks
them against ANTLR on generated sentences and mutants. It also checks `fileseq.g4`,
whose whitespace is a token rather than skipped: its nodes' tokens are
contiguous in the source, the case where `GetText` returns a slice of it.
