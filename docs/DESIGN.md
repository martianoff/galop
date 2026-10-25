# galop design

This is the working design behind the [roadmap](ROADMAP.md): how a `.g4` grammar
becomes a parser, and the contracts between the packages that do it.

## Pipeline

```
.g4 text ──g4.Parse──▶ g4.Grammar ──g4.Check──▶ (diagnostics)
                           │
                     atn.Compile
                           ▼
        atn.Compiled { Vocab, Parser ATN + rule IR, Lexer ATN }
              │                                │
      analysis.Analyze                    lexgen (DFA)
              ▼                                ▼
        decision plans  ───────▶  codegen.Target (Go) ──▶ files
```

| Package | Role |
|---|---|
| `g4` | `.g4` reader: lexer, parser, model, formatter, semantic checks |
| `atn` | Token numbering (ANTLR-compatible), the ATN, the parser rule IR |
| `analysis` | FIRST/FOLLOW, static LL(k) per decision, adaptive decisions, conflict report |
| `lexgen` | Lexer ATN → DFA with ANTLR's lexer semantics |
| `codegen` | The `Target` interface and the target registry |
| `golang` | The Go target |
| `cmd/galop` | The command-line tool, with ANTLR's options and message format |

Each stage is a pure function of the previous stage's output, so stages that do
not depend on each other run concurrently (see [Concurrency](#concurrency)).

## Compatibility with ANTLR

galop's contract is "the same grammar means the same thing as in ANTLR 4":

- **Token types** are numbered as ANTLR numbers them (`atn/vocab.gala`): implicit
  literal tokens `T__n` in order of first use in parser rules, then non-fragment
  lexer rules in file order, then `tokens {}` names, then names only referenced.
  Generated code can therefore consume tokens from an ANTLR-generated lexer and
  vice versa.
- **Decisions and alternative numbers** come from an ATN built the way ANTLR builds
  it (`atn/build.gala`): an optional block's alternatives and its exit share one
  decision; a loop has its own enter/exit (or repeat/exit) decision; a non-greedy
  quantifier puts the exit first; a block of single-token alternatives is a set, not
  a decision.
- **Prediction** picks the alternative ANTLR's prediction picks on every input ANTLR
  parses without a syntax error. Two mechanisms, chosen per decision by the analysis:
  1. *Static lookahead.* If the alternatives' lookahead languages (computed with
     global FOLLOW, i.e. any calling context) are disjoint within K ≤ 3 tokens, the
     decision is a `switch` on `LA(1)`, then `LA(2)`… Only one alternative can be
     viable on that lookahead in any context, so this agrees with ANTLR.
  2. *Adaptive prediction*, for the lookahead prefixes static analysis cannot settle.
     Generated parsers keep a stack of return states (one `int32` push per rule call)
     and, at such a decision, run a small ATN simulator over the static ATN tables
     with the real call stack: ANTLR's full-context LL prediction, including its
     stopping rule (stop when every conflicting subset of configurations resolves to
     the same minimum alternative) and its graph-structured context merging. There
     is no per-process warm-up: the tables are Go literals, and only decisions the
     report lists ever reach the simulator.
- **Trees.** The default Go flavor (below) builds ANTLR's own context objects, so
  trees print identically with `antlr.TreesStringTree`.
- **Lexing** follows ANTLR's lexer: longest match, the earlier rule wins a tie,
  non-greedy loops stop at the first exit, recursive fragments work, `skip`, `more`,
  `type(...)`, `channel(...)`, modes.

The differential harness (`differential/`) checks all of this against ANTLR
4.13.1 on the GALA corpus.

## The ATN and the rule IR (`atn`)

`atn.Compile(g)` returns `atn.Compiled`:

- `Vocab`: token names, literals, implicit flags, indexed by token type.
- `Parser`: the parser ATN. `States[i].Out` is ordered; for a decision state the
  i-th transition starts alternative i+1. `Decisions[d]` names the decision's
  state, rule, kind (`AltBlock`, `OptionalBlock`, `StarEntry`, `PlusBack`),
  greediness, alternative count and source text.
- `Rules`: the rule IR, one `RuleIR` per parser rule. It is the rule as a tree of
  `Step`s (`MatchToken`, `MatchSet`, `CallRule`, `Choose`, `Loop`, `RunAction`,
  `CheckPredicate`) annotated with decision ids and, for each rule call, the ATN
  follow state. Code generators walk the IR; the analysis works on the ATN.
- `Lexer`: the lexer ATN, rules in ANTLR's lexer order (implicit `T__n` rules first),
  `TokensStart` with one alternative per token rule, `RuleTokenType` per rule.
  Lexer commands sit on `LexerActions` transitions at the end of an alternative.

## Analysis (`analysis`)

`analysis.Analyze(parserATN, maxK) Analysis` (types in `analysis/plan.gala`):

- `First`, `Follow`, `Nullable` per rule.
- One `DecisionPlan` per decision: its `Lookahead` tree (`Predict`, `Switch` on
  `LA(depth)`, `Adaptive` for the residue), its class (`LL1`, `LLk`,
  `NeedsAdaptive`) and, for the report, example conflicting lookahead prefixes.

Lookahead sets are computed by walking the ATN from each alternative with an
explicit call stack; reaching a rule's stop state with an empty stack continues at
every caller's follow state (global FOLLOW). EOF is token -1 and is in FOLLOW of
the start rule(s) (rules no other rule calls). Decisions are independent of each
other and are analyzed concurrently.

The **conflict report** (`galop -report`) lists every decision that is not
statically decided, with its rule, source text, the alternatives in conflict and an
example lookahead. It is the grammar lint the roadmap describes.

## Lexer (`lexgen`)

Builds a DFA from the lexer ATN by subset construction over ordered configuration
lists (ATN state, alternative/rule, rule-call stack, passed-non-greedy flag, lexer
actions), reproducing ANTLR's `LexerATNSimulator`: closure runs in ANTLR's order,
the first configuration to reach a token rule's stop state decides the token (rule
order breaks ties), an alternative that has accepted stops its configurations that
passed a non-greedy decision, lexer commands travel with the configuration (and are
ignored inside a called rule), and the longest accepted prefix wins.

- **Alphabet.** Code points are partitioned into classes that every transition
  treats alike (62 for `gala.g4`); DFA edges are per class.
- **Recursive fragments.** Rule-call stacks are unbounded (`BRACE_CONTENT` calls
  itself), so the static DFA keeps stacks up to a depth (`lexerDepth`, default 12:
  274 states for `gala.g4`, about 8 more per level). An edge that would need a
  deeper stack is *dynamic*: the generated lexer then re-matches that token on the
  ATN, from the token's start, with a small simulator over emitted ATN tables. The
  answer is ANTLR's at any depth; only deeply nested tokens pay for it.
- **Reference.** `DFA.Tokenize` runs the DFA the way `BaseLexer.NextToken` runs
  ANTLR's simulator (skip, more, type, channel, EOF, error recovery). The unit
  tests use it, and the generated Go lexer follows it.
- **Not supported yet:** lexer modes, predicates and embedded actions in lexer
  rules (reported as errors), and `EOF` inside lexer rules.

ANTLR's Go runtime compares two lexer configurations with no actions as equal
whenever their hashes collide, whatever their states; galop compares them in full.
This only matters on a 32-bit hash collision inside one configuration set.

### The Go lexer (companion flavor)

`lexgen.GoLexerFile` writes `<grammar>_galop_lexer.go` into the package of ANTLR's
generated code. The type embeds `*antlr.BaseLexer` and implements `antlr.Lexer`, so
it replaces ANTLR's lexer without an adapter: a token source that wraps an
`antlr.Lexer` (as GALA's line-break rule does), `antlr.CommonTokenStream`, error
listeners and token factories all work unchanged. Tokens are created by the lexer's
token factory (`*antlr.CommonToken` by default) with ANTLR's type, channel,
code-point start/stop, line and column; after each token the input stream sits
where ANTLR's lexer leaves it, so `GetInputStream().LA(1)` still reads the next
character. Token recognition errors go to the error listeners with ANTLR's message
and position, and recovery skips the same characters.

A standalone token source is not an option: `antlr.TokenSource` has an unexported
method, so only a type embedding an antlr type can implement it.
As in ANTLR's lexer, the line and column live in `Interpreter`, a
`*antlr.LexerATNSimulator`; callers may set its `Line` and `CharPositionInLine`
before the first token (GALA does for `ParseExpressionAt`). That simulator only
counts: it holds no ATN, and `GetATN()` returns nil.

The lexer scans the input's text directly with tables indexed by class (an ASCII
table, binary search above it). All tables are immutable package-level data, so
lexers on different goroutines share no state and need no cache isolation.

## Code generation (`codegen`, `golang`)

`codegen.Target` (in `codegen/target.gala`) is the extension point for languages:
`Language()` matches `-Dlanguage=…` and `Generate(Input, Options)` returns
`OutputFile`s. The Go target has **flavors**, chosen with `-Dflavor=…`:

- **`companion`** (first): generates `<grammar>_galop_parser.go` and
  `<grammar>_galop_lexer.go` into the same Go package as ANTLR's own output for the
  grammar. The fast parser constructs ANTLR's generated context types
  (`NewXxxContext`, labeled-alternative subtypes, label setters) and consumes an
  `antlr.TokenStream`; the fast lexer produces `antlr.CommonToken`s. Existing code
  that walks ANTLR trees keeps working unchanged, and ANTLR's parser remains
  available as a fallback (for example for error recovery). This is how the GALA
  compiler adopts galop.
- **`standalone`** (later): generates the context types, listener and visitor too,
  on a small galop runtime with ANTLR-shaped accessors and no ANTLR dependency.

Generated parsers are safe to use from many goroutines at once: all tables are
immutable package-level data and all parse state lives in the parser instance.

## Command line (`cmd/galop`)

`galop` behaves like the `antlr4` tool:

```
galop [options] grammar1.g4 grammar2.g4 ...
  -o <dir>              output directory
  -lib <dir>            where to find imported grammars and .tokens files
  -package <name>       package of the generated code
  -listener / -no-listener, -visitor / -no-visitor
  -D<option>=<value>    set a grammar option, e.g. -Dlanguage=Go, -Dflavor=companion
  -Xexact-output-dir    write all output to -o, ignoring the grammar's directory
  -depend               print the files that would be generated
  -message-format antlr|gnu|vs2005
  -Werror               treat warnings as errors
  -atn                  write each rule's ATN as a Graphviz .dot file
galop-only:
  -report               print the decision report
  -check [-roundtrip]   validate only, write nothing
  -format               print the grammar in canonical form
  -version
```

Messages use ANTLR's format and, where the problem is the same, ANTLR's error
numbers: `error(56): gala.g4:12:5: reference to undefined rule: foo`. Problems only
galop has (a construct it does not support yet) use numbers from 500 up. The exit
code is 1 when any error was reported.

## Bazel (`rules_gala`)

`rules_gala` provides `galop_parser(name, grammar, package, flavor, ...)`, the
counterpart of the usual ANTLR genrule: it runs galop on a `.g4` and yields the
generated Go sources for a `go_library`.

## Concurrency

galop is written in GALA and uses `concurrent.Future`:

- the CLI processes grammar files concurrently;
- for one grammar, parser analysis and lexer DFA construction run concurrently;
- decisions are analyzed concurrently; rules are emitted concurrently and joined
  in rule order, so output is deterministic.

Generated parsers have no shared mutable state, so callers can parse files in
parallel without the cache isolation ANTLR's Go runtime needs.
