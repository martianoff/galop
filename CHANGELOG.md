# Changelog

Changes that affect galop's users, newest first. Each release on GitHub also
lists every merged pull request.

## Unreleased

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
