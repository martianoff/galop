# Changelog

Changes that affect galop's users, newest first. Each release on GitHub also
lists every merged pull request.

## 0.8.0

- **Added:** the native Python parser: `-Dflavor=native` writes
  `<grammar>_parser.py` next to the lexer, whose `parse_<rule>(tokens)` builds
  ANTLR's tree with no runtime. Decisions galop settles ahead of time read
  their lookahead; the others, and left-recursive rules' precedence, are
  predicted as ANTLR's Python runtime predicts them (LL mode, an SLL DFA
  cached across parses). It is several times faster than ANTLR's Python
  parser on the corpus grammars. Syntax errors raise `ParseError`, not yet
  with ANTLR's message. A grammar with actions or predicates gets a parser
  module that raises `NotImplementedError`, saying why, when imported.
- **Added:** the Python target's `native` flavor lexer (`-Dflavor=native`):
  `<grammar>_lexer.py`, a module with no dependency whose `lex(src)` returns
  ANTLR's tokens as arrays, with off-channel tokens and token recognition
  errors as ANTLR's Python runtime words them, in about a sixth of ANTLR's
  time on the GALA corpus. It is the first step of the native Python parser.
- **Added:** `EOF` in lexer rules (`('\n' | EOF)`), which matches the end of
  the input without consuming it, as in ANTLR's lexer, in the Go and Python
  lexers. galop reported it as error 56.

## 0.7.0

- **Added:** a Python target, `-Dlanguage=Python3`, which writes lexers so
  far: `<grammar>GalopLexer.py` subclasses the ANTLR Python runtime's `Lexer`
  (4.13.1) and stands in for ANTLR's generated lexer in front of ANTLR's
  parser, with the same tokens, errors and class attributes. On the GALA
  corpus it lexes in about a third of ANTLR's time. `//differential/python`
  checks it against ANTLR's Python lexer on the corpus grammars.
- **Added:** ANTLR's error 94, a named action (`@members`) defined twice in one
  scope, and warning 108, a `tokens {}` name that is already a token, with
  ANTLR's positions and counts, in grammars that import others too. ANTLR's
  quirks are kept: a combined grammar's repeated action is reported twice
  (ANTLR checks it in its lexer and in its parser), and an action without a
  scope takes the scope of the last action that has one.
- **Added:** ANTLR's error 54, a second `options`, `import` or `tokens`
  section. As in ANTLR, only a grammar's first `import` statement is loaded,
  and an imported grammar contributes only its first `tokens {}` and
  `options {}` sections; galop merged them all.
- **Fixed:** token types are numbered as ANTLR numbers them when lexer rules
  have `type(...)` or `more` commands: such a rule gets a type of its own only
  if it is exactly one literal, after the other rules, and a lexer grammar's
  `tokens {}` names come first. galop numbered every rule in file order, so the
  Go target's `antlr` flavor could hand ANTLR's parser the wrong token types.
  A parser rule that uses the name of a rule that defines no token gets
  ANTLR's warning 125.
- **Fixed:** literal names are ANTLR's: `'\''` is named `'''` in the generated
  `LiteralNames` and in error messages, as in ANTLR's generated code.
- **Fixed:** a `tokens {}` name of an implicit literal token (`T__0`) is that
  token; the generated Go code declared its constant twice.

## 0.6.0

- **Added:** grammar imports, `import A, B = C;`, as ANTLR 4.13.1 resolves and
  merges them: an imported grammar is looked for in the working directory, next
  to the importing grammar and in `-lib`; the root grammar's rules win; ANTLR's
  import errors and warning (11, 109, 110, 111, 113) are reported with its
  wording and positions. Where galop's diagnostics differ is in
  [DESIGN.md](docs/DESIGN.md#grammar-imports).
- **Added:** nikunjy/rules' `JsonQuery.g4`, which imports `SemverGrammar.g4`, to
  the corpus, checked against ANTLR by `//differential/convert`.
- **Fixed:** a name listed twice in `tokens {}` (or by a grammar and one it
  imports) is one token, as in ANTLR. The generated Go code declared its
  constant twice and did not compile.
- **Fixed:** in the native flavor, `GetText` on a tree converted from ANTLR's
  error-recovered parse could include a token that is in no node. ANTLR's
  recovery can consume an error token into a left-recursive rule's context
  and then discard that context. Such trees now set `Dropped`, and `GetText`
  builds their text from the nodes' tokens.

## 0.5.2

- **Changed (license):** the code galop generates is yours to use under any terms:
  `LICENSE` now ends with an exception that frees the generated lexers, parsers,
  listeners, visitors and converters (the runtime code from galop's templates
  included) from the Apache License's conditions. galop itself stays under the
  Apache License, Version 2.0.
- **Fixed:** the corpus grammars copied from other projects now come with the
  copyright and license notices their licenses require
  (`testdata/grammars/NOTICE.md`).

## 0.5.1

- **Changed:** in the native flavor, `GetText` returns a slice of the source, with no
  copy, when a node's tokens are contiguous in it (no skipped or hidden text
  between them, valid UTF-8, no tokens made up by error recovery); otherwise it
  builds the text as before. On gofileseq's file-sequence grammar, whose
  whitespace is a token, that is 3 to 5 fewer allocations per parsed path.
- **Added:** gofileseq's `fileseq.g4` to the corpus, checked against ANTLR by
  `//differential/convert`; the convert tests compare every node's `GetText`
  with ANTLR's.
- **Fixed:** generated Go files are formatted as gofmt formats them: the Go target
  runs them through `go/format`, so `gofmt -l` and editors no longer flag them
  (a few templates left fields and constants unaligned).

## 0.5.0

- **Changed:** directly left-recursive rules are generated by default, in both
  flavors, with error recovery and the converter: they are no longer
  experimental. `-Xleft-recursion`, which turned them on in 0.4.0, is still
  accepted and has no effect. A grammar with such a rule no longer gets error 500
  from code generation.
- **Changed:** the native flavor's lexer and `Parse` functions collect token lists
  and build trees in arrays they keep between calls, and return copies of exactly
  their size. Trees were sized for about four nodes per token and token lists for
  a token per four bytes, so a short input allocated several times what it kept:
  on Milvus's filter expressions a parse now allocates 840 bytes instead of 1,772,
  and on 32 goroutines, where the garbage collector had set the pace, throughput
  went from about 1.6M to 2.7M expressions/s on one 32-thread machine. Token
  lists from the lexer now have no spare capacity.

## 0.4.0

- **Added (experimental):** directly left-recursive rules
  (`e : e '*' e | e '+' e | INT`) are rewritten into a precedence loop as
  ANTLR 4.13.1 rewrites them: the same classification of alternatives (binary,
  prefix, suffix, other), precedences, loop order, precedence arguments and
  labels. `-report` shows the rewritten rule. With the experimental
  `-Xleft-recursion` option, both flavors generate such rules, with ANTLR's
  precedence predicates in prediction, error recovery and the converter; the
  corpus now holds four grammars that use them (Milvus's filter expressions,
  CEL, SigNoz's having expressions, Grule), checked against ANTLR. Without the
  option, code generation still rejects them (error 500). A left-recursive rule
  with no other alternative is ANTLR's error 147.
- **Added:** `<assoc=left>` and `<assoc=right>` at the start of an alternative;
  another `assoc` value is ANTLR's warning 84.

- **Changed (breaking):** the antlr flavor is a drop-in for ANTLR's parser. Its
  rule methods have ANTLR's signatures (`p.Prog()` returns `IProgContext`): on
  input with syntax errors they report every error ANTLR reports to the
  parser's error listeners and return the context ANTLR's default error
  strategy builds, its error nodes, made-up tokens, label fields and
  exceptions included. The galop parser embeds ANTLR's generated parser for its
  error listeners and the rest of ANTLR's parser API, and passes it to the
  contexts it makes, as ANTLR's code does. The former methods, which return
  the first error and no tree, are now `Parse<Rule>()` (`p.ParseProg()`).
  galop's ATN state numbers differ from ANTLR's, so `GetState()` and invoking
  states are galop's; `GetExpectedTokens()` is answered by galop.

- **Added:** each error of the native flavor's `Parse<Rule>WithRecovery` carries
  `Expected`, the token types ANTLR's `GetExpectedTokens()` gives an error
  listener at that error, so code that builds "expecting one of ..." messages
  from ANTLR's listener can move to the native flavor.
- **Changed:** the native flavor's `Parse` functions reuse parsers from a pool,
  keeping their buffers between parses (on Milvus's filter expressions, 10
  allocations per parse instead of 16). Where the first-error replay finds only
  the alternatives ANTLR's prediction falls back on, the answer is cached by
  entry rule and token types (at most 4,096 inputs of at most 1,024 tokens).
- **Fixed:** code generation and `-report` no longer run without end on a
  directly left-recursive rule with more than one left-recursive alternative
  (`e : e '*' e | e '+' e | INT`); they report error 500 at the rule, as for
  other constructs galop does not support yet.
- **Fixed:** a lexer mode (`mode M;`) is reported as not supported yet (error
  500), not as a syntax error.

## 0.3.3

- **Fixed:** for input that is not valid UTF-8, the native flavor's token text is now
  ANTLR's: `Token.Text`, `Tokens.Text`, `SourceText` and token recognition error
  messages give U+FFFD for each byte that is not part of a valid encoding (a
  multibyte sequence cut short included), as ANTLR's input stream reads it. Token
  spans and `Tokens.Src` are unchanged: they stay on the input's bytes. On such
  input `Text` now differs from `Src[Start:End]`, which still gives the raw bytes.
- **Added:** `Tokens.Texts` and `Tokens.HiddenTexts`, each token's ANTLR text for
  input that is not valid UTF-8 (nil otherwise), which the lexer and the converter
  fill; `Tokens.HiddenText(i)`, a comment's text, the counterpart of `Tokens.Text`;
  and `Tokens.ReadTexts()`, which fills the texts for token lists built elsewhere.
  `Tokens.Text` and `HiddenText` read them with no check: prefer them to
  `Token.Text`, which checks the text on each call.

## 0.3.2

- **Fixed:** parsing a rule that does not end with EOF (an expression parsed on its
  own) gives ANTLR's LL-mode tree. Such a parse now runs in LL mode from its first
  pass: an SLL pass could succeed with the lowest alternative of a conflict that
  ends the rule early, where ANTLR's full-context prediction prefers the one that
  reads further (`a {} b` as an expression is the composite literal `a {}`, not
  `a`). And where ANTLR's prediction finds no viable alternative but falls back on
  one that had already left the decision's rule, the parse takes it too and can
  stop early without an error (`a {}; b` is `a`), instead of reporting "no viable
  alternative"; `Parse<Rule>WithRecovery` then returns that tree and no errors.
  Parses of rules that end with EOF give the same results as before.

## 0.3.1

- **Added:** `<Grammar>ParseError.Read` and `<Grammar>Tree.Read`, how far ANTLR's
  parse had read when it reported an error and over the whole parse, and the native
  lexer's `Tokens.ErrorAt`, the token after each token recognition error. Together
  they place token recognition errors among syntax errors in the order ANTLR's lazy
  lexer reports them, and drop those ANTLR never reports.

## 0.3.0

- **Added:** error recovery in the native flavor. `Parse<Rule>WithRecovery` and
  `<Grammar>ParseWithRecovery(tokens, rule)` recover from syntax errors as ANTLR's
  default error strategy does: they return every syntax error ANTLR reports, in
  order (`<Grammar>ParseError`, with the offending token and the node of the rule
  being parsed), and ANTLR's error-recovered tree, as the converter converts it.
  Without a syntax error they return `Parse<Rule>`'s tree.
- **Fixed:** the first-error replay decides a `*` or `+` loop whose body is a block
  of alternatives with one token where ANTLR's generated code does, rather than
  predicting it. No first error changed on the corpora.

## 0.2.5 and earlier

- **Added:** `<Grammar>ToNativeContexts`, the antlr flavor's converter that also
  returns the ANTLR context of each native node, by node index.
- **Changed:** in the native flavor, the getter of a non-list label that matches
  more than once in a node (`a=ID ',' a=ID`, or a label inside a loop) returns its
  last match, as ANTLR's does; it returned the first. The tree still records every
  match.
- **Added:** the antlr flavor's converter to the native tree
  (`-DnativePackage=<native package>` writes `<grammar>_galop_native.go`), for
  ANTLR's trees, error-recovered ones included.
- **Added:** native error tokens and conjured tokens, `Kind()`, `Typed()`, a
  generated listener with `Walk` (by default) and a visitor (`-visitor`).
- **Fixed:** a native `Parse<Rule>` entry function is no longer renamed when its
  rule shares a name with a handle method (`ParseRef` stays `ParseRef`).
