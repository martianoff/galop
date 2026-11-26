"""Checks galop's native Python parser against ANTLR's Python parser.

Usage: parser_test.py <grammar name> <native base> <start rule> <samples>
[<corpus manifest>].
ANTLR's modules are <grammar name>Lexer and <grammar name>Parser; galop's are
<native base>_lexer and <native base>_parser. The inputs are the galop-sample
sentences in <samples>, mutants of them (tokens' text inserted, removed or
swapped), the sentences joined in pairs and, with a manifest, every file it
lists. For each input ANTLR's parser runs as it does by default (LL
prediction, the default error strategy): where it reports no syntax error,
galop's parse_<start rule> must give the same tree (ANTLR's toStringTree);
where it reports one, galop's must raise ParseError with ANTLR's first error:
its line, column and message.

ANTLR's Python runtime leaks one thing from a parse with a syntax error into
later parses: a sync at a loop back adds its recovery set to the ATN's cached
nextTokens set of that state (getExpectedTokens returns the cached set, and
IntervalSet.addSet changes it in place), which changes later syncs and
messages. galop reports what ANTLR reports without that leak, so the test
clears the cache after each input ANTLR reports an error on (see the Python
target's differences in galop's docs/DESIGN.md).
"""

import importlib
import random
import sys
import time

from antlr4 import CommonTokenStream, InputStream
from antlr4.error.ErrorListener import ErrorListener
from python.runfiles import runfiles


class Counter(ErrorListener):
    """Counts the parser's syntax errors and keeps the first."""

    def __init__(self):
        self.errors = 0
        self.first = None

    def syntaxError(self, recognizer, symbol, line, column, msg, e):
        if self.first is None:
            self.first = (line, column, msg)
        self.errors += 1


def samples(path):
    """The sentences of a galop-sample file: each is its length in bytes on a
    line, then its text and a newline."""
    with open(path, "rb") as f:
        data = f.read()
    out, i = [], 0
    while i < len(data):
        j = data.index(b"\n", i)
        n = int(data[i:j])
        out.append(data[j + 1 : j + 1 + n].decode("utf-8"))
        i = j + 1 + n + 1
    return out


def mutants(sentences, native, rnd):
    """Each sentence with one token's text removed, duplicated or swapped
    with the next one's, so that many of them are not in the language."""
    out = []
    for s in sentences:
        ts = native.lex(s)
        n = len(ts.types) - 1
        if n < 1:
            continue
        spans = [(ts.starts[i], ts.ends[i]) for i in range(n)]
        i = rnd.randrange(n)
        a, b = spans[i]
        out.append(s[:a] + s[b:])
        out.append(s[:b] + " " + s[a:b] + s[b:])
        if i + 1 < n:
            c, d = spans[i + 1]
            out.append(s[:a] + s[c:d] + s[b:c] + s[a:b] + s[d:])
    return out


def main(argv):
    r = runfiles.Create()
    grammar, base, start, sample_path = argv[:4]
    lexer_cls = getattr(importlib.import_module(grammar + "Lexer"), grammar + "Lexer")
    parser_cls = getattr(importlib.import_module(grammar + "Parser"), grammar + "Parser")
    native_lexer = importlib.import_module(base + "_lexer")
    native = importlib.import_module(base + "_parser")
    parse = getattr(native, "parse_" + start)

    sentences = samples(r.Rlocation(sample_path))
    inputs = [("sample %d" % i, s) for i, s in enumerate(sentences)]
    inputs += [("mutant %d" % i, s) for i, s in enumerate(mutants(sentences, native_lexer, random.Random(1)))]
    inputs += [("pair %d" % i, a + "\n" + b) for i, (a, b) in enumerate(zip(sentences[::2], sentences[1::2]))]
    if len(argv) > 4:
        with open(r.Rlocation(argv[4]), encoding="utf-8") as f:
            for line in f:
                name, location = line.rstrip("\n").split("\t")
                with open(r.Rlocation(location), encoding="utf-8") as src:
                    inputs.append((name, src.read()))

    problems = []
    times = [0.0, 0.0]
    valid = 0
    for name, text in inputs:
        t0 = time.perf_counter()
        parser = parser_cls(CommonTokenStream(lexer_cls(InputStream(text))))
        parser.removeErrorListeners()
        counter = Counter()
        parser.addErrorListener(counter)
        # ANTLR's Python target escapes a rule named like a reserved word
        # (input_).
        tree = getattr(parser, start + "_" if hasattr(parser, start + "_") else start)()
        want = tree.toStringTree(recog=parser)
        t1 = time.perf_counter()
        try:
            got = parse(native_lexer.lex(text)).to_string_tree()
            error = None
        except native.ParseError as e:
            got, error = None, e
        times[0] += t1 - t0
        times[1] += time.perf_counter() - t1
        if counter.errors == 0:
            valid += 1
            if got != want:
                problems.append("%s %r:\n  ANTLR %s\n  galop %s" % (name, text[:80], want, got if error is None else error))
        elif error is None:
            problems.append("%s %r: ANTLR reports %d syntax errors, galop none:\n  galop %s" % (name, text[:80], counter.errors, got))
        elif (error.line, error.column, error.msg) != counter.first:
            problems.append("%s %r: different first errors:\n  ANTLR %d:%d %s\n  galop %d:%d %s"
                            % ((name, text[:80]) + counter.first + (error.line, error.column, error.msg)))
        if counter.errors:
            # Undo the runtime's leak (see the module's docstring).
            for st in parser.atn.states:
                st.nextTokenWithinRule = None
    print("%d inputs (%d valid), %d problems; ANTLR %.2fs, galop %.2fs"
          % (len(inputs), valid, len(problems), times[0], times[1]))
    for p in problems[:10]:
        print(p)
    if len(problems) > 10:
        print("... %d in all" % len(problems))
    return 1 if problems or valid == 0 else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
