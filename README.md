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

```sh
galop check grammar.g4               # parse and validate
galop check --roundtrip *.g4         # also verify format → parse gives the same grammar
galop format grammar.g4              # print the grammar in canonical form
```

`check` reports syntax errors with their position and semantic errors:
undefined or duplicate rules, lexer rules that reference parser rules, parser
rules that reference fragments.

`format` prints a canonical form. Comments are not part of the grammar model,
so they are not printed.

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
./galop check --roundtrip testdata/grammars/*.g4 # the grammar corpus
bazel test //...                                 # the same suite under Bazel
```

## License

[Apache 2.0](LICENSE)
