"""Checks galop's Python lexers against ANTLR's, token by token.

Usage: lexer_test.py <ANTLR lexer module> <galop lexer module> <galop native
lexer module> <samples> [<corpus manifest>]. The first two modules each
declare a class of their own name; the native module has lex(). The inputs
are the galop-sample sentences in <samples>, mutants of them (code
points inserted, inputs cut short), the sentences joined into one input, a
few edge cases and, with a manifest, every file it lists. For each input both
lexers must give the same tokens (type, channel, start, stop, line, column,
text, through EOF) and the same token recognition errors, and the native
lexer the same tokens split into default-channel and hidden ones, each error
with the index of the first default-channel token after it. The class
attributes ANTLR's lexer declares must be galop's too, and its constants the
native module's.
"""

import importlib
import random
import sys
import time

from antlr4 import InputStream
from antlr4.error.ErrorListener import ErrorListener
from python.runfiles import runfiles


class Recorder(ErrorListener):
    def __init__(self, tokens):
        self.tokens = tokens
        self.errors = []
        self.error_at = []

    def syntaxError(self, recognizer, symbol, line, column, msg, e):
        self.errors.append((line, column, msg))
        # ANTLR's lexer reports an error while producing the next token.
        self.error_at.append(sum(1 for t in self.tokens if t[1] == 0))


def lex(cls, text):
    """ANTLR-style lexing with cls: the tokens, the errors and, for each
    error, how many default-channel tokens came before it."""
    lexer = cls(InputStream(text))
    lexer.removeErrorListeners()
    tokens = []
    recorder = Recorder(tokens)
    lexer.addErrorListener(recorder)
    while True:
        t = lexer.nextToken()
        tokens.append((t.type, t.channel, t.start, t.stop, t.line, t.column, t.text))
        if t.type == -1:
            return tokens, recorder.errors, recorder.error_at


def split(lexed):
    """ANTLR's tokens as the native lexer gives them: default-channel tokens
    with [start, end) spans, hidden ones with the index of the next
    default-channel token, the errors, and where each error falls."""
    tokens, errors, error_at = lexed
    default, hidden = [], []
    for t in tokens:
        if t[1] == 0 or t[0] == -1:
            default.append((t[0], t[2], t[3] + 1, t[4], t[5]))
        else:
            hidden.append((t[0], t[2], t[3] + 1, t[4], t[5], t[1], len(default)))
    return default, hidden, errors, error_at


def lex_native(module, text):
    ts = module.lex(text)
    default = list(zip(ts.types, ts.starts, ts.ends, ts.lines, ts.columns))
    return default, [tuple(h) for h in ts.hidden], [(e.line, e.column, e.msg) for e in ts.errors], ts.error_at


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
    native = importlib.import_module(argv[2])
    problems = []
    for attr in ["literalNames", "symbolicNames", "ruleNames", "channelNames", "modeNames", "grammarFileName"]:
        if getattr(antlr, attr) != getattr(galop, attr):
            problems.append("%s: ANTLR %r, galop %r" % (attr, getattr(antlr, attr), getattr(galop, attr)))
    for k, v in vars(antlr).items():
        if isinstance(v, int) and not k.startswith("_") and getattr(galop, k, None) != v:
            problems.append("constant %s: ANTLR %r, galop %r" % (k, v, getattr(galop, k, None)))
        native_name = k + "_" if k in ("Token", "Tokens", "HiddenToken", "LexError") else k
        if isinstance(v, int) and not k.startswith("_") and k not in ("DEFAULT_MODE", "MORE", "SKIP", "DEFAULT_TOKEN_CHANNEL", "HIDDEN", "MIN_CHAR_VALUE", "MAX_CHAR_VALUE") and getattr(native, native_name, None) != v:
            problems.append("native constant %s: ANTLR %r, galop %r" % (k, v, getattr(native, native_name, None)))

    path = r.Rlocation(argv[3])
    if path.endswith("_inputs.txt"):
        # One input per line, with Python's backslash escapes.
        with open(path, encoding="ascii") as f:
            sentences = [line.rstrip("\n").encode("ascii").decode("unicode_escape") for line in f]
    else:
        sentences = samples(path)
    files = []
    if len(argv) > 4:
        with open(r.Rlocation(argv[4]), encoding="utf-8") as f:
            for line in f:
                name, location = line.rstrip("\n").split("\t")
                with open(r.Rlocation(location), encoding="utf-8") as src:
                    files.append((name, src.read()))
    inputs = [("sample %d" % i, s) for i, s in enumerate(sentences)]
    inputs += [("mutant %d" % i, s) for i, s in enumerate(mutants(sentences, random.Random(1)))]
    inputs += [("joined", "\n".join(sentences)), ("empty", ""), ("newline", "\n")]
    inputs += [("one code point " + repr(c), c) for c in "\"'/\\{@$\x00é😀"]
    inputs += files

    times = [0.0, 0.0, 0.0]
    mismatches = 0
    native_mismatches = 0
    for name, text in inputs:
        t0 = time.perf_counter()
        a = lex(antlr, text)
        t1 = time.perf_counter()
        b = lex(galop, text)
        t2 = time.perf_counter()
        c = lex_native(native, text)
        times[0] += t1 - t0
        times[1] += t2 - t1
        times[2] += time.perf_counter() - t2
        want = split(a)
        if c != want:
            native_mismatches += 1
            if native_mismatches <= 5:
                problems.append("native %s %r:" % (name, text[:80]))
                for what, x, y in zip(["tokens", "hidden", "errors", "error_at"], want, c):
                    if x != y:
                        diff = next(((u, v) for u, v in zip(x, y) if u != v), (x[len(y):len(y) + 1], y[len(x):len(x) + 1]))
                        problems.append("  %s: ANTLR %r, galop %r" % (what, diff[0], diff[1]))
        if a[:2] != b[:2]:
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
    print("%d inputs (%d files), %d mismatches, %d native; ANTLR %.2fs, galop %.2fs, native %.2fs"
          % (len(inputs), len(files), mismatches, native_mismatches, times[0], times[1], times[2]))
    if mismatches > 5:
        problems.append("... %d inputs differ in all" % mismatches)
    if native_mismatches > 5:
        problems.append("... %d inputs differ in all for the native lexer" % native_mismatches)
    for p in problems:
        print(p)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
