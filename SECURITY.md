# Security Policy

## Reporting a vulnerability

**Do not open a public issue or pull request for a security problem.**

Report privately through GitHub: go to
<https://github.com/martianoff/galop/security/advisories/new> (repository → **Security**
→ **Report a vulnerability**), describe the issue and how to reproduce it.

Useful to include: the grammar or input that triggers the problem, the exact `galop`
command, `galop version`, your OS and architecture, and the impact as you see it.

## What counts

galop reads grammars and, as the roadmap progresses, generates parsers that other
programs run on untrusted input. Examples of what we treat as vulnerabilities:

- a generated parser that crashes, hangs or uses unbounded memory or time on some input
  (for example exponential backtracking), where the grammar itself does not call for it;
- generated code that does something other than parse (writes files, runs commands);
- `galop` itself executing code from a grammar file.

A grammar file's embedded `{...}` actions are copied into generated code by design, so a
grammar is trusted input to the generator; its *parsers* must not trust their input.

## Supported versions

galop is pre-1.0. Fixes land on `main` and ship in the next release.
