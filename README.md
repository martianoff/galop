# galop

**galop** is a parser generator for ANTLR-style `.g4` grammars, written in
[GALA](https://github.com/martianoff/gala).

It reads the grammar you already have and works out every parsing decision when
it generates the parser, not while the parser runs. The output is plain
recursive-descent code: no runtime library, no grammar interpreter, no warm-up.

> **Status: early development.** galop reads, validates and formats `.g4`
> grammars today. Grammar analysis and code generation are next; see the
> [roadmap](docs/ROADMAP.md).

## Why

ANTLR's runtime interprets a serialized grammar and builds its prediction
tables while it parses, once per process, and falls back to full-context
prediction when a decision is ambiguous. For a compiler that runs as a
short-lived CLI, that warm-up is most of the parse. Measured on the GALA
compiler's own grammar (Go runtime, a 1,100-line source file):

| | ANTLR 4 (Go runtime) | Hand-written recursive descent |
|---|---|---|
| Warm parse | 7.4 ms | 0.74 ms |
| First parse in a fresh process | 88 ms | ~0.5 ms |
| A file that needs full-context prediction | 150 ms | 0.37 ms |

galop's goal is to generate parsers in that second column from an unchanged
`.g4` file. Its first user is the GALA compiler itself.

## Install

Build from source with the [GALA CLI](https://github.com/martianoff/gala/releases):

```sh
git clone https://github.com/martianoff/galop
cd galop
gala build -o galop ./cmd/galop
```

Tagged releases publish binaries for Linux, macOS and Windows.

## Usage

galop is used like ANTLR's `antlr4` tool: the same options, the same output
layout, the same message format.

```sh
galop -o gen -package parser Expr.g4      # generate Go code into gen/
galop -depend -o gen Expr.g4              # list the files it would write
galop -check -roundtrip grammars/*.g4     # validate only; also check the formatter keeps each grammar
galop -format Expr.g4                     # print the grammar in canonical form
galop -report Expr.g4                     # list the decisions static lookahead cannot settle
```

```
galop 0.0.0-dev, a parser generator for ANTLR 4 grammars
usage: galop [options] grammar.g4...
 -o ___              specify output directory where all output is generated
 -lib ___            specify location of grammars, tokens files
 -atn                generate rule augmented transition network diagrams
 -message-format ___ specify output style for messages in antlr, gnu, vs2005
 -listener           generate parse tree listener (default)
 -no-listener        don't generate parse tree listener
 -visitor            generate parse tree visitor
 -no-visitor         don't generate parse tree visitor (default)
 -package ___        specify a package/namespace for the generated code
 -depend             generate file dependencies
 -D<option>=value    set/override a grammar-level option
 -Werror             treat warnings as errors
 -Xexact-output-dir  all output goes into -o dir regardless of paths/package
galop options:
 -check              validate grammars without generating anything
 -roundtrip          with -check, also verify that formatting keeps each grammar
 -format             print each grammar in canonical form
 -report             print the decisions static lookahead cannot settle
 -version            print the galop version
 -help               print this help
```

- **Targets.** `-Dlanguage=Go` (the default) picks the Go target, and
  `-Dflavor=companion|standalone` its output style (see
  [design](docs/DESIGN.md#code-generation-codegen-golang)). `-D` options override
  the grammar's own `options {}`.
- **Output layout** follows ANTLR: files go next to the grammar; with `-o dir`
  they go to `dir` plus the grammar's relative directory (`-o gen sub/T.g4`
  writes to `gen/sub/`), unless `-Xexact-output-dir` is given.
- **`-atn`** writes each rule's network as a Graphviz file, `T.rule.dot` (a
  combined grammar's lexer rules are `TLexer.RULE.dot`), as ANTLR does.
- **Messages** use ANTLR's numbers and wording where the problem is the same
  one ANTLR reports, and numbers from 500 up for galop's own (such as a
  construct it does not support yet). `-message-format` picks ANTLR's layouts:

  ```
  error(56): Expr.g4:4:33: reference to undefined rule: factor                  # antlr (default)
  grammars/Expr.g4:4:33: error: reference to undefined rule: factor [error 56]  # gnu
  grammars/Expr.g4(4,33) : error 56 : reference to undefined rule: factor       # vs2005
  ```

  As in ANTLR, the column is the 0-based character position, and the `antlr`
  layout shows only the file name. The exit code is 1 when any error was
  reported; `-Werror` makes warnings errors.
- **`-check`** parses, checks and compiles each grammar: syntax errors,
  undefined or redefined rules, lexer rules that reference parser rules,
  parser rules that reference fragments, undecodable literals and sets, and
  (a warning, as in ANTLR) tokens used but never defined.
- **`-format`** prints a canonical form. Comments are not part of the grammar
  model, so they are not printed.

`-lib` is accepted; importing grammars or a token vocabulary (`import`,
`tokenVocab`) is not supported yet and is reported as an error.

Grammar files are processed concurrently and independently: an error in one
does not stop the others, and output is reported in command-line order.

### Bazel

[rules_gala](https://github.com/martianoff/rules-gala) provides
`galop_parser`, which runs galop on a `.g4` file and yields the generated Go
sources for a `go_library`.

## Supported `.g4` syntax

- `grammar`, `lexer grammar`, `parser grammar` headers
- `options { ... }`, `tokens { ... }`, `channels { ... }`, `@name { ... }` and `@scope::name { ... }` actions
- parser and lexer rules, `fragment` rules, alternatives with `# Label`
- rule and token references, quoted literals, `'a'..'z'` ranges, `[...]` sets, `.`, `~`
- `?` `*` `+` and their non-greedy forms `??` `*?` `+?`
- element labels `x=e` and `xs+=e`
- embedded actions `{...}` and predicates `{...}?`
- lexer commands: `-> skip`, `-> channel(HIDDEN)`, `-> pushMode(X)`, ...

Not supported yet, and reported as errors rather than ignored: rule arguments,
`returns`/`locals`, rule-level options and actions, element options
(`<assoc=right>`), lexer modes, grammar imports.

## Development

See [AGENTS.md](AGENTS.md) for the project rules and [CONTRIBUTING.md](CONTRIBUTING.md)
for the workflow.

```sh
gala test                                        # unit tests
gala build -o galop ./cmd/galop                  # the CLI
./galop -check -roundtrip testdata/grammars/*.g4 # the grammar corpus
bazel test //...                                 # the same suite under Bazel
```

## License

[Apache 2.0](LICENSE)
