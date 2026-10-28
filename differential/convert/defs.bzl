"""convert_test: galop's generated code and converter, checked for one grammar."""

load("@rules_go//go:def.bzl", "go_library", "go_test")

_PREFIX = "github.com/martianoff/galop/differential/convert/"

def convert_test(name, grammar, grammar_name, start, sample_flags, extra_srcs = []):
    """Generates and tests one grammar's parsers and converter.

    It generates ANTLR 4.13.1's Go parser and galop's antlr flavor with the
    converter (package `<name>/antlr`), galop's native flavor with its
    listener and visitor (`<name>/native`) and sentences from galop-sample,
    and runs convert_test.go.tmpl over them as `<name>_test`.

    Args:
      name: the test's name, also the directory of the generated packages.
      grammar: the .g4 file's label.
      grammar_name: the name the grammar declares (`grammar Expr;`).
      start: the entry rule the sentences start from.
      sample_flags: galop-sample's flags (count, seed, nesting, length).
      extra_srcs: more test files for this grammar, in the same package.
    """
    lower = grammar_name.lower()
    entry = start[0].upper() + start[1:]
    antlr_dir = name + "/antlr"
    native_dir = name + "/native"

    native.genrule(
        name = name + "_antlr_go",
        srcs = [grammar],
        outs = [antlr_dir + "/" + lower + f for f in ["_lexer.go", "_parser.go"]],
        cmd = """
            set -e
            out=$(RULEDIR)/{dir}_out
            "$(JAVA)" -jar $(location @antlr4_tool//jar) -Dlanguage=Go -package antlr -no-listener \
                -Xexact-output-dir -o "$$out" $(location {grammar})
            cp "$$out"/*.go $(RULEDIR)/{dir}/
            rm -rf "$$out"
        """.format(dir = antlr_dir, grammar = grammar),
        toolchains = ["@bazel_tools//tools/jdk:current_host_java_runtime"],
        tools = ["@antlr4_tool//jar"],
    )

    native.genrule(
        name = name + "_galop_go",
        srcs = ["//cmd/galop", grammar],
        outs = [antlr_dir + "/" + lower + f for f in ["_galop_lexer.go", "_galop_native.go", "_galop_parser.go"]],
        cmd = "$(location //cmd/galop) -Dlanguage=Go -Dflavor=antlr -DnativePackage={native} -package antlr -Xexact-output-dir -o $(RULEDIR)/{dir} $(location {grammar})".format(
            native = _PREFIX + native_dir,
            dir = antlr_dir,
            grammar = grammar,
        ),
    )

    native.genrule(
        name = name + "_native_go",
        srcs = ["//cmd/galop", grammar],
        outs = [native_dir + "/" + lower + f for f in ["_lexer.go", "_parser.go"]],
        cmd = "$(location //cmd/galop) -visitor -package native -Xexact-output-dir -o $(RULEDIR)/{dir} $(location {grammar})".format(
            dir = native_dir,
            grammar = grammar,
        ),
    )

    go_library(
        name = name + "_native",
        srcs = [":" + name + "_native_go"],
        importpath = _PREFIX + native_dir,
    )

    go_library(
        name = name + "_antlr",
        srcs = [":" + name + "_antlr_go", ":" + name + "_galop_go"],
        importpath = _PREFIX + antlr_dir,
        deps = [
            ":" + name + "_native",
            "@com_github_antlr4_go_antlr_v4//:antlr",
        ],
    )

    native.genrule(
        name = name + "_samples",
        srcs = ["//cmd/galop-sample", grammar],
        outs = [name + "_samples.txt"],
        cmd = "$(location //cmd/galop-sample) {flags} -start {start} -o $@ $(location {grammar})".format(
            flags = sample_flags,
            start = start,
            grammar = grammar,
        ),
    )

    native.genrule(
        name = name + "_test_go",
        srcs = ["convert_test.go.tmpl"],
        outs = [name + "_test.go"],
        cmd = "sed -e 's|@G@|{g}|g' -e 's|@START@|{entry}|g' -e 's|@ANTLRPKG@|{a}|g' -e 's|@NATIVEPKG@|{n}|g' $< > $@".format(
            g = grammar_name,
            entry = entry,
            a = _PREFIX + antlr_dir,
            n = _PREFIX + native_dir,
        ),
    )

    go_test(
        name = name + "_test",
        size = "small",
        srcs = [":" + name + "_test_go"] + extra_srcs,
        args = ["-samples=$(rlocationpath :" + name + "_samples)"],
        data = [":" + name + "_samples"],
        deps = [
            ":" + name + "_antlr",
            ":" + name + "_native",
            "//differential/corpus",
            "@com_github_antlr4_go_antlr_v4//:antlr",
        ],
    )
