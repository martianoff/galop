"""py_lexer_test: galop's Python lexer checked against ANTLR's for one grammar."""

load("@pypi//:requirements.bzl", "requirement")
load("@rules_python//python:defs.bzl", "py_library", "py_test")

def py_lexer_test(name, grammar, grammar_name, sample_flags = None, inputs = None, antlr_module = None, defines = [], imports = [], corpus = None):
    """Generates both Python lexers of a grammar and compares them.

    It generates ANTLR 4.13.1's Python lexer and galop's in both flavors, and
    sentences from galop-sample, and runs lexer_test.py over them as
    `<name>_test`: every sentence, mutants of them and, with corpus, every file
    it lists, must give the same tokens and errors from all three lexers. A lexer grammar, which
    galop-sample cannot write sentences for, gives its own inputs instead.

    Args:
      name: the test's name, also the prefix of the generated targets.
      grammar: the .g4 file's label.
      grammar_name: the name the grammar declares (`grammar Expr;`).
      sample_flags: galop-sample's flags (count, seed, nesting, length).
      inputs: instead of sentences from galop-sample, which needs parser
        rules, a file of inputs: one per line, with Python's backslash
        escapes.
      antlr_module: the module and class of ANTLR's lexer, when not
        `<grammar_name>Lexer` (a lexer grammar's is its own name).
      defines: -D options for galop (`lexerDepth=2`).
      imports: the grammars the grammar imports, next to it.
      corpus: a corpus_manifest target whose files are lexed too.
    """
    antlr_module = antlr_module or grammar_name + "Lexer"
    base = grammar_name[:-len("Lexer")] if grammar_name.endswith("Lexer") and grammar_name != "Lexer" else grammar_name
    galop_module = base + "GalopLexer"
    native_module = base.lower() + "_lexer"

    native.genrule(
        name = name + "_antlr_py",
        srcs = [grammar] + imports,
        outs = [name + "/" + antlr_module + ".py"],
        cmd = """
            set -e
            out=$(RULEDIR)/{name}_antlr_out
            "$(JAVA)" -jar $(location @antlr4_tool//jar) -Dlanguage=Python3 -no-listener \
                -Xexact-output-dir -o "$$out" $(location {grammar})
            cp "$$out"/{module}.py $@
            rm -rf "$$out"
        """.format(name = name, grammar = grammar, module = antlr_module),
        toolchains = ["@bazel_tools//tools/jdk:current_host_java_runtime"],
        tools = ["@antlr4_tool//jar"],
    )

    native.genrule(
        name = name + "_galop_py",
        srcs = ["//cmd/galop", grammar] + imports,
        outs = [name + "/" + galop_module + ".py"],
        cmd = "$(location //cmd/galop) -Dlanguage=Python3 {defines} -Xexact-output-dir -o $(RULEDIR)/{name} $(location {grammar})".format(
            defines = " ".join(["-D" + d for d in defines]),
            name = name,
            grammar = grammar,
        ),
    )

    native.genrule(
        name = name + "_native_py",
        srcs = ["//cmd/galop", grammar] + imports,
        outs = [name + "/" + native_module + ".py"],
        cmd = "$(location //cmd/galop) -Dlanguage=Python3 -Dflavor=native {defines} -Xexact-output-dir -o $(RULEDIR)/{name} $(location {grammar})".format(
            defines = " ".join(["-D" + d for d in defines]),
            name = name,
            grammar = grammar,
        ),
    )

    samples = inputs or ":" + name + "_samples"
    if not inputs:
        native.genrule(
            name = name + "_samples",
            srcs = ["//cmd/galop-sample", grammar] + imports,
            outs = [name + "_samples.txt"],
            cmd = "$(location //cmd/galop-sample) {flags} -o $@ $(location {grammar})".format(
                flags = sample_flags,
                grammar = grammar,
            ),
        )

    py_library(
        name = name + "_lexers",
        srcs = [":" + name + "_antlr_py", ":" + name + "_galop_py", ":" + name + "_native_py"],
        imports = [name],
    )

    args = [antlr_module, galop_module, native_module, "$(rlocationpath " + samples + ")"]
    data = [samples]
    if corpus:
        args.append("$(rlocationpath " + corpus + ")")
        data.append(corpus)

    py_test(
        name = name + "_test",
        size = "medium" if corpus else "small",
        srcs = ["lexer_test.py"],
        main = "lexer_test.py",
        args = args,
        data = data,
        deps = [
            ":" + name + "_lexers",
            requirement("antlr4-python3-runtime"),
            "@rules_python//python/runfiles",
        ],
    )
