# Fuzzing and the performance gate

Two harnesses guard the generated parsers.

- **Fuzzing** checks that galop parses exactly like ANTLR on input no corpus holds.
- **The performance gate** checks that a change does not make galop slower or
  make it predict more at runtime.

Both run on `gala.g4` in the [differential harness](../differential/README.md).
The input generator works for any grammar galop accepts.

```mermaid
flowchart LR
  g["gala.g4"] --> s["galop-sample<br/>(fixed seed)"]
  s --> v["generated sentences<br/>(valid code)"]
  c["GALA corpus"] --> m["token mutator<br/>(fixed seed)"]
  v --> m
  m --> i["mutants<br/>(mostly invalid code)"]
  v --> p["parity tests:<br/>both flavors vs ANTLR"]
  i --> p
  c --> p
  v --> q["perfgate"]
  i --> q
  c --> q
  q --> b{"vs baseline.json"}
  b -- "counters: exact" --> r["report / fail"]
  b -- "ratios: tolerance" --> r
```

## Generated input: `galop-sample`

`galop-sample` (package [`sample`](../sample/sample.gala)) writes random sentences of a
grammar's parser rule.

**How it builds a sentence:**

1. It walks the grammar's parser ATN.
2. At each choice it takes an alternative that can still finish the rule. Half the
   time that is the alternative it has taken least often from that state, so rare
   alternatives come up too: a tuple pattern after `case`, a composite literal, a
   lambda. Otherwise it picks uniformly.
3. Once a rule is active more than `-nesting` times at once, or the sentence passes
   `-tokens` tokens, it takes the shortest way out. Every sentence ends.

**How it writes each token:** it walks that token's lexer rule and keeps a text only
if the grammar's own lexer reads it back as exactly that token. A token type with no
such text is never generated: for example, a `tokens {}` name that no lexer rule
produces, such as GALA's `NL_LPAREN`.

```sh
galop-sample [-n count] [-seed n] [-nesting n] [-tokens n] [-start rule] -o out grammar.g4
```

- **Deterministic.** The same grammar, flags and seed give the same file on every
  platform: the random source is SplitMix64, not Go's.
- **Output format.** Each sentence is written as its byte length on its own line,
  then the sentence and a newline.

The Bazel targets `//differential:samples`, `:samples_deep` and `:samples_large_1`
to `_4` generate fixed sets from `gala.g4`:

| Target | Sentences | Nesting | Max tokens | Used by |
|---|---:|---:|---:|---|
| `samples` | 600 | 2 | 300 | `generated_test`, `perfgate` |
| `samples_deep` | 200 | 4 | 800 | `generated_deep_test` |
| `samples_large_1` … `_4` | 4 × 1,000 | 3 | 600 | `generated_large_1_test` … `_4_test` (run on main) |

Keep `-nesting` at 4 or below for `gala.g4`. Lambdas and if-expressions nest without
brackets, and at six levels some 2 KB sentences take ANTLR's own prediction minutes,
against 0.1 s for galop. The oracle would time out, not galop.

## Parity tests

The tests are `//differential/diff:generated_test` and `:generated_deep_test`, run on
every PR, plus `:generated_large_1_test` to `_4_test`, run on pushes to main. The
large set is split in four because ANTLR's caches grow with every deep sentence, to
about 7 GB per test process. Each one runs three
checks on its sample set, for both flavors:

- **`TestGeneratedMatchesANTLR`** runs every generated sentence:
  - same verdict as ANTLR (the GALA compiler's SLL-then-LL strategy);
  - on accepted input, ANTLR's tree;
  - on rejected input, ANTLR's first syntax error: the same message at the same line
    and column.
  - A sentence with a lexical error fails the test: `galop-sample` writes only token
    texts the lexer reads back, so that is a generator bug. (`lexdiff` checks the
    lexers themselves.)
- **`TestMutations`** checks mutated copies of the generated sentences: a token
  dropped, doubled or preceded by stray text, or brackets changed. It checks the
  verdict and tree, and that a rejected file reports the same error as ANTLR's
  whole-input LL rerun.
- **`TestSyntaxErrors`** checks that rejected mutants report ANTLR's first error.

These are the same checks the corpus mutation tests run (`diff_test`, `heldout_test`
and the large runs), pointed at the generated input with `-samples` instead of
`-manifest`.

The converter from ANTLR's trees to the native tree (see
[design](DESIGN.md#from-antlrs-tree-to-the-native-tree)) has its own checks, run on
both corpora and both generated sets:

- **`TestConvertMatchesNative`** converts ANTLR's tree and galop's antlr-flavor tree
  of every input native parsing accepts, and requires native parsing's tree: the same
  tokens, hidden tokens, nodes, child references, token labels and `Next`, with no
  made-up tokens.
- **`TestConvertRecoveredTrees`** converts ANTLR's error-recovered trees of rejected
  mutants (`-recovered_trees`, 100 per PR target and 500 to 2,000 in the large runs).
  Each must be well formed: children before parents, every node but the root a
  child exactly once, real tokens in order and at most once, spans in bounds. It
  must also print as ANTLR prints the recovered tree.
- **`//differential/convert`** builds every test grammar (Expr, Calc, Labels, Errors,
  Repair) with ANTLR, the antlr flavor with the converter and the native flavor with
  its listener and visitor. It runs the two checks above on sentences `galop-sample`
  generates and on mutants of them, and walks every tree with `Typed`, `Accept` and
  `Walk`. Labels.g4 holds the labels ANTLR's fields keep only the last match of.
- **`TestTypedVisitorListener`** (`diff_test`) checks `Typed`, `GalaAccept` and
  `GalaWalk` on the corpus trees and on recovered trees. Each node must get its
  rule's or labeled alternative's handle, and the walk must report the order
  ANTLR's `ParseTreeWalker` does.

```sh
bazel test //differential/diff:generated_test //differential/diff:generated_deep_test
bazel test //differential/diff:generated_large_1_test   # one of the four large runs (manual)
```

## The performance gate: `perfgate`

`bazel run //differential/cmd/perfgate` measures three fixed inputs:

- **corpus:** every third file of the GALA corpus (241 files);
- **generated:** `//differential:samples`;
- **mutants:** 100 mutants of the corpus subset (seed 1).

### Counters

Counters are deterministic, so they are gated exactly. Each is measured on one
goroutine in a fresh process, so the process-wide caches start empty. They are taken
for both flavors.

| Counter | Meaning |
|---|---|
| `predictions` | runtime (adaptive) predictions, in every pass: the first, the repair or LL passes, and the replay that finds a rejected file's first error |
| `full-context` | full-context (LL) predictions |
| `replayed-files` | files that needed more than one pass (repair, rerun over tokens) |
| `replays-median`, `replays-max` | extra passes per such file; the maximum is the worst file, so a rise there fails even when the median holds |
| `sll-states`, `full-context-states` | states the prediction caches built: the warm-up |
| `sll-peak-states`, `sll-resets` | the most states the bounded SLL cache held, and how many times it started over (0 below its bound) |
| `allocs-per-pass` | allocations of a warm pass |
| `files`, `accepted` | the input's size and how much of it parses (reported, not judged: the parity tests check acceptance) |

The generated parser counts its own work in plain fields (`PFXStats`), which cost no
synchronization. The harness reads them through `nativegala.ParseWithStats` and
`antlrgala.ParseGalopWithStats`.

A counter above its baseline fails the gate. Allocations are the one exception: the
Go runtime's own background allocations move them by a few, so the baseline's
rules give them 1% (see [The baseline's rules](#the-baselines-rules)).

### Wall-clock times

Three operations are timed on 1 and on 32 goroutines:

- the native flavor;
- ANTLR, running the GALA compiler's strategy;
- the native lexer alone, which is the floor every parse pays.

Each is timed both warm and as the first pass in a fresh process.

Absolute times depend on the machine and are only reported, so the gate judges
ratios measured in the same job:

- **`…/galop-vs-antlr`:** galop's time over ANTLR's.
- **`…/galop-vs-lexer`:** galop's time over the lexer's warm time. The lexer's first
  pass is too short to time well.

The ratios on 32 goroutines are only reported. The CI runner has four cores, and two
runs of the same code differed by up to a third there. Ratios on one goroutine moved
by up to 25%, so they are judged within 30%.

To keep the ratios stable on a shared runner:

- The three operations are timed in rounds, one sample of each per round. Each ratio
  is the median over rounds of that round's ratio, so a slow stretch slows both sides
  alike.
- Before timing, two untimed passes fill the caches. A warm pass is then timed to
  work out how many passes make a 50 ms sample (rounded up), and each sample runs
  passes until it has lasted at least 50 ms.
- A ratio fails only when it exceeds its baseline by more than its rule's tolerance.

The gaps (first pass minus warm) are reported for galop and for the lexer. The
lexer's gap is the floor any Go program pays.

`…/w1/convert/warm-ms` is the warm time of converting each input's ANTLR trees to
the native tree on one goroutine (the antlr flavor's tree where it accepts the file,
ANTLR's recovered one where it does not). It is only reported.

### CI

| Where | What | Judged |
|---|---|---|
| every PR and push: job `performance gate` | `perfgate` with the defaults above (about 2 minutes once built) | against `baseline.json`; enforcing (`PERF_ENFORCE: "true"` in `ci.yml`): a regression or a missing number fails the job |
| pushes to main and `workflow_dispatch`: jobs `fuzz (…)` | the large fuzz runs, one runner each: corpus mutants, held-out mutants, generated sets 1–4 | parity with ANTLR |
| pushes to main and `workflow_dispatch`: job `performance sweep` | `perfgate -every=1 -mutants=300 -runs=7 -passes=7` | not judged: a wider report |

A cold cache never turns a pull request red. The gate's build and measurement run
under a time budget (8 minutes on a pull request), and when they run past it the job
skips the gate with a notice and passes.

The gate writes its tables to the job summary and its numbers to the `perf` artifact
(`perf.json`).

### Running it locally

```sh
bazel run //differential/cmd/perfgate                    # report against the baseline
bazel run //differential/cmd/perfgate -- -times=false    # counters only (about 20 s)
bazel run //differential/cmd/perfgate -- -enforce        # exit 1 on a regression
```

On a busy machine the ratios are noisy, so judge ratios from CI runs. The counters
are the same everywhere.

### Updating the baseline

The baseline is [`differential/perf/baseline.json`](../differential/perf/baseline.json).
Update it when a change is meant to move the numbers, for example a change that
removes runtime predictions, and say so in the PR:

```sh
bazel run //differential/cmd/perfgate -- -update=differential/perf/baseline.json
```

- **Counters** come out the same on any machine.
- **Ratios** come from the CI runner (`ubuntu-latest`), never from a local machine:
  they depend on it. Download the `perf` artifact of a green run on the PR and copy its
  `ratios` into the baseline.
- `-update` keeps the baseline's rules. With `-times=false` it measures counters only
  and keeps the baseline's ratios.

### The baseline's rules

How each number is judged is policy, kept in the baseline file rather than in code.
`rules` is a list tried in order; the first whose `pattern` (a `path.Match` pattern,
`*` matching one `/`-separated part of the name) matches a number judges it:

```json
"rules": [
  {"pattern": "*/*/files", "tolerance": 0, "report_only": true},
  {"pattern": "*/*/accepted", "tolerance": 0, "report_only": true},
  {"pattern": "*/*/allocs-per-pass", "tolerance": 0.01},
  {"pattern": "*/w32/*/*", "tolerance": 0, "report_only": true},
  {"pattern": "*/*/*/*", "tolerance": 0.3}
]
```

- A number may exceed its baseline by `tolerance` (0.3 allows 30%) before it is
  `worse`.
- A `report_only` number is shown and never fails the gate. `files` and `accepted`
  are report-only because the parity tests already fail on any difference from ANTLR
  in what parses; fewer accepted files is a correctness failure there, not a speed-up.
- A number the baseline has but the run doesn't measure is `missing`, which fails
  like a regression: a renamed or dropped counter can't slip through.
- A number no rule matches has tolerance 0: counters (`flavor/input/name`, three
  parts) are judged exactly, and the last rule matches only ratios (four parts).

A verdict of `better` means a number fell below its baseline by more than its
tolerance. It doesn't fail the gate, but the baseline should be tightened to it so
the gain is kept.
