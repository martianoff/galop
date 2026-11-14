# Changelog

Changes that affect galop's users, newest first. Each release on GitHub also
lists every merged pull request.

## Unreleased

- **Fixed:** for input that is not valid UTF-8, the native lexer's token text is now
  ANTLR's: each byte that is not part of a valid encoding, and each byte of a
  multibyte sequence cut short at the end, is U+FFFD, as ANTLR's input stream reads
  it. `<Grammar>LexAt` decodes such input first, so `Tokens.Src` and token spans
  refer to the decoded text; pass `Tokens.Src`, not the original input, to
  `Token.Text`. The converter's `<Grammar>ToNative` does the same. Valid input is
  not copied.

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
