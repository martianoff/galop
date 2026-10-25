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
| `codegen` | The `Target` interface; `cmd/galop` lists the targets a binary ships |
| `golang` | The Go target |
| `diag` | Diagnostics: ANTLR's error numbers and message formats, galop's own codes |
| `tool` | The command line: argument parsing, output layout, the per-file pipeline |
| `cmd/galop` | `main`: wires the targets and the file system into `tool` |

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
  2. *Adaptive prediction*, for the decisions static lookahead cannot settle. This is
     ANTLR's own prediction (its Go runtime 4.13.1, quirks included) run over the
     ATN, which generated parsers carry as Go literal tables:
     - **SLL first.** The calling context is a wildcard: a rule's end continues
       after every call of the rule. Prediction stops at a unique alternative or at
       an SLL conflict (`hasSLLConflictTerminatingPrediction`), which SLL settles by
       the lowest conflicting alternative. The analysis builds as much of each
       decision's SLL DFA as it can ahead of time (`Automaton`); states beyond that
       are computed at runtime and cached for the life of the process, as ANTLR
       caches them. Walking cached edges takes no lock.
     - **LL on retry.** A parse runs in SLL mode; if it hits a syntax error it is
       retried from the start in LL mode, where an SLL conflict runs full-context
       prediction with the parser's real call stack (generated parsers push one
       `int32` return state per rule call) until ANTLR's stopping rule picks an
       alternative. This is how the GALA compiler drives ANTLR, and SLL success gives
       the LL tree on the whole corpus.
     - **Contexts** are graph-structured stacks, hash-consed and merged with ANTLR's
       rules (`merge`, `mergeSingletons`, `mergeArrays`, wildcard root in SLL), so
       configuration sets, conflict subsets and DFA states are exactly ANTLR's.
       Tracking each stack separately is not just slower: with global FOLLOW the
       number of distinct stacks grows combinatorially.
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

On input it parses, ANTLR's LL prediction picks the smallest alternative that can
parse the rest of the input; it stops as soon as every group of configurations
with the same state and full stack has the same smallest alternative. The
analysis applies that rule statically, on groups that exist at runtime either
all or none in every calling context: configurations still inside the decision's
rule with equal state and stack, and configurations that left the rule after the
same number of tokens (they all passed through the same runtime configuration,
the rule's end with the caller's stack). So a true ambiguity such as
`typeAlias : identifier | type` resolves statically to alternative 1 after the
identifier, as ANTLR resolves it. A decision whose alternatives start with a
predicate is always adaptive: ANTLR evaluates those predicates while predicting.

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
  -version, -help
```

The README lists the options exactly as `galop -help` prints them.

**Diagnostics** (`diag`) carry a code, a severity, a file and a 1-based line and
column. They are rendered in ANTLR's three layouts (`antlr`, `gnu`, `vs2005`),
byte for byte as ANTLR lays them out, including its 0-based column and the
double space where a tool error has no location:
`error(56): gala.g4:12:4: reference to undefined rule: foo`,
`error(7):  cannot find or open file: x.g4`. Where the problem is one ANTLR
reports, the code and wording are ANTLR 4.13.1's (`diag/kinds.gala` lists them);
problems only galop has use codes from 500 up:

| Code | Problem |
|---|---|
| 500 | a construct galop does not support yet (imports, `tokenVocab`, rule arguments, ...) |
| 501 | a parser rule references a fragment rule |
| 502 | `-check -roundtrip`: the formatter does not preserve the grammar |
| 503 | an element that cannot appear in this kind of rule (a `[set]` in a parser rule) |
| 504 | a set that matches nothing |
| 505 | the target rejected the grammar |
| 506 | a command line galop cannot act on (missing value, conflicting actions, no files) |
| 507 | a reversed range in a lexer set (`[z-a]`) |

Two places follow ANTLR where galop used to be stricter: a token a parser rule
uses but nothing defines is warning 125 (ANTLR then defines it, and so does
`atn.BuildVocabulary`), and an unknown `-D` or grammar option is warning 83. The
exit code is 1 when any error was reported; `-Werror` adds ANTLR's one-time
error 10 after the first warning and stops that grammar before code generation.

**Positions.** The grammar model records where the grammar name, each option,
each rule name and each element start (`Rule.Located`, in the pre-order of
`Rule.Nodes`). They place diagnostics; they do not change what a grammar means,
so equality that matters (the formatter round trip) compares
`g4.StripPositions` of both sides. `atn.Compile` places its errors at the
element being compiled.

**Pipeline** (`tool`). `tool.Run(args, Toolkit)` is the whole command line;
`main` only wires a `Toolkit` (the targets, the file system, the version) and
prints the `Outcome`. A target is added to `main`'s list; there is no global
registry. For each grammar file the front end (read, parse, check, compile,
analyze) runs in its own `Future`; generation and writes then run in
command-line order, so output and diagnostics are deterministic and an error
in one grammar does not stop the others.

## Bazel (`rules_gala`)

`rules_gala` provides `galop_parser(name, grammar, package, flavor, kind, ...)`,
the counterpart of the usual ANTLR genrule: it runs galop on a `.g4` and
yields the generated Go sources (declared outputs named like ANTLR's Go
target) for a `go_library`.

rules_gala gets galop from a **toolchain of prebuilt release binaries**, not by
building galop from source with `gala_binary`. The GALA compiler is built from
source in Bazel and will generate its own parser with galop; building galop
from source inside `galop_parser` would need the GALA toolchain, which needs
the GALA compiler, which needs its parser: a cycle. A prebuilt binary breaks it,
and users who only want Go parsers need neither Go nor GALA toolchains for the
generator. galop's own repository (or anyone else) can still register a
`galop_toolchain` pointing at a from-source build.

## Concurrency

galop is written in GALA and uses `concurrent.Future`:

- the CLI runs each grammar file's front end (parse, check, compile, analyze)
  concurrently. Generation and writes stay on the main goroutine: GALA's
  data-race check never lets an interface or a function value (an injected
  target or file system) cross into a `Future`;
- for one grammar, parser analysis and lexer DFA construction run concurrently;
- decisions are analyzed concurrently; rules are emitted concurrently and joined
  in rule order, so output is deterministic.

Generated parsers have no shared mutable state, so callers can parse files in
parallel without the cache isolation ANTLR's Go runtime needs.
