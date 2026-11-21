"""Checks galop's Python lexer against ANTLR's, token by token.

Usage: lexer_test.py <ANTLR lexer module> <galop lexer module> <samples>
[<corpus manifest>]. The modules each declare a class of their own name. The
inputs are the galop-sample sentences in <samples>, mutants of them (code
points inserted, inputs cut short), the sentences joined into one input, a
few edge cases and, with a manifest, every file it lists. For each input both
lexers must give the same tokens (type, channel, start, stop, line, column,
text, through EOF) and the same token recognition errors. The class
attributes ANTLR's lexer declares must be galop's too.
"""

import importlib
import random
import sys
import time

from antlr4 import InputStream
from antlr4.error.ErrorListener import ErrorListener
from python.runfiles import runfiles


class Recorder(ErrorListener):
    def __init__(self):
        self.errors = []

    def syntaxError(self, recognizer, symbol, line, column, msg, e):
        self.errors.append((line, column, msg))


def lex(cls, text):
    lexer = cls(InputStream(text))
    lexer.removeErrorListeners()
    recorder = Recorder()
    lexer.addErrorListener(recorder)
    tokens = []
    while True:
        t = lexer.nextToken()
        tokens.append((t.type, t.channel, t.start, t.stop, t.line, t.column, t.text))
        if t.type == -1:
            return tokens, recorder.errors


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


def mutants(inputs, rnd):
    noise = list("$#@`~\\\"'\n\r\t \x00é€→😀{}[]()/*-+=<>!?.,;:|&^%09azAZ_")
    out = []
    for s in inputs:
        cs = list(s)
        for _ in range(3):
            cs.insert(rnd.randint(0, len(cs)), rnd.choice(noise))
        out.append("".join(cs))
        if len(s) > 1:
            out.append(s[: rnd.randint(1, len(s) - 1)])
    return out


def main(argv):
    r = runfiles.Create()
    antlr = getattr(importlib.import_module(argv[0]), argv[0])
    galop = getattr(importlib.import_module(argv[1]), argv[1])
    problems = []
    for attr in ["literalNames", "symbolicNames", "ruleNames", "channelNames", "modeNames", "grammarFileName"]:
        if getattr(antlr, attr) != getattr(galop, attr):
            problems.append("%s: ANTLR %r, galop %r" % (attr, getattr(antlr, attr), getattr(galop, attr)))
    for k, v in vars(antlr).items():
        if isinstance(v, int) and not k.startswith("_") and getattr(galop, k, None) != v:
            problems.append("constant %s: ANTLR %r, galop %r" % (k, v, getattr(galop, k, None)))

    path = r.Rlocation(argv[2])
    if path.endswith("_inputs.txt"):
        # One input per line, with Python's backslash escapes.
        with open(path, encoding="ascii") as f:
            sentences = [line.rstrip("\n").encode("ascii").decode("unicode_escape") for line in f]
    else:
        sentences = samples(path)
    files = []
    if len(argv) > 3:
        with open(r.Rlocation(argv[3]), encoding="utf-8") as f:
            for line in f:
                name, location = line.rstrip("\n").split("\t")
                with open(r.Rlocation(location), encoding="utf-8") as src:
                    files.append((name, src.read()))
    inputs = [("sample %d" % i, s) for i, s in enumerate(sentences)]
    inputs += [("mutant %d" % i, s) for i, s in enumerate(mutants(sentences, random.Random(1)))]
    inputs += [("joined", "\n".join(sentences)), ("empty", ""), ("newline", "\n")]
    inputs += [("one code point " + repr(c), c) for c in "\"'/\\{@$\x00é😀"]
    inputs += files

    times = [0.0, 0.0]
    mismatches = 0
    for name, text in inputs:
        t0 = time.perf_counter()
        a = lex(antlr, text)
        t1 = time.perf_counter()
        b = lex(galop, text)
        times[0] += t1 - t0
        times[1] += time.perf_counter() - t1
        if a != b:
            mismatches += 1
            if mismatches <= 5:
                problems.append("%s %r:" % (name, text[:80]))
                for x, y in zip(a[0], b[0]):
                    if x != y:
                        problems.append("  first different token: ANTLR %r, galop %r" % (x, y))
                        break
                if len(a[0]) != len(b[0]):
                    problems.append("  ANTLR %d tokens, galop %d" % (len(a[0]), len(b[0])))
                if a[1] != b[1]:
                    problems.append("  errors: ANTLR %r, galop %r" % (a[1][:3], b[1][:3]))
    print("%d inputs (%d files), %d mismatches; ANTLR %.2fs, galop %.2fs"
          % (len(inputs), len(files), mismatches, times[0], times[1]))
    if mismatches > 5:
        problems.append("... %d inputs differ in all" % mismatches)
    for p in problems:
        print(p)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
