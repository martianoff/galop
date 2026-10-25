# Contributing to galop

Thanks for your interest. galop is early; the [roadmap](docs/ROADMAP.md) says what is
being built and in which order, and issues are the place to discuss a change before a
large PR.

## Setup

You need the [GALA CLI](https://github.com/martianoff/gala/releases) at the version in
`gala.mod`, Go 1.25+, and optionally [Bazelisk](https://github.com/bazelbuild/bazelisk)
for the Bazel build.

```sh
gala test                                        # unit tests
gala build -o galop ./cmd/galop                  # the CLI
./galop -check -roundtrip testdata/grammars/*.g4 # the grammar corpus
bazel test //...                                 # the same suite under Bazel
bazel run //:gazelle                             # after adding or removing a .gala file
```

## Workflow

1. Branch from `main`.
2. Make the change with tests (see the testing rules in [AGENTS.md](AGENTS.md#testing)).
3. Run the pre-commit checklist in [AGENTS.md](AGENTS.md#pre-commit-checklist).
4. Open a PR using the template, and keep the description true to the diff.
5. CI must be green on the final commit.

If you hit a GALA compiler bug, report it in [martianoff/gala](https://github.com/martianoff/gala/issues)
with a minimal repro rather than working around it in galop.

## AI-assisted contributions

**Contributions made with AI coding agents are welcome**, including PRs written largely
or entirely by an agent. Three things are required:

1. **A human is accountable.** Whoever opens the PR has read the whole diff, understands
   it, has run the checks, and will answer review questions about it.
2. **Disclose it.** Tick the AI-assistance box in the PR template, and add a
   `Co-Authored-By:` trailer naming the tool or model to commits the agent wrote.
   Disclosure is not a mark against a PR; it tells the reviewer where to look harder.
3. **The agent follows [AGENTS.md](AGENTS.md).** It is the single rulebook for every
   agent; `CLAUDE.md` only points to it.

A PR whose description doesn't match the diff, that weakens tests to pass, or that works
around a GALA bug instead of reporting it, is closed without detailed review.

## License

By contributing you agree that your contributions are licensed under the
[Apache 2.0 license](LICENSE).
