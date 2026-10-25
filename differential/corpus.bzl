"""corpus_manifest lists a set of source files for the harness tools."""

def _corpus_manifest_impl(ctx):
    lines = []
    for f in ctx.files.srcs:
        # The runfiles location of an external file is its short_path without
        # the leading "../"; the display name also drops the repository.
        if f.short_path.startswith("../"):
            rlocation = f.short_path[len("../"):]
            name = rlocation.split("/", 1)[1]
        else:
            rlocation = ctx.workspace_name + "/" + f.short_path
            name = f.short_path
        lines.append(name + "\t" + rlocation)
    out = ctx.actions.declare_file(ctx.label.name + ".tsv")
    ctx.actions.write(out, "\n".join(sorted(lines)) + "\n")
    return [DefaultInfo(
        files = depset([out]),
        runfiles = ctx.runfiles(files = [out] + ctx.files.srcs),
    )]

corpus_manifest = rule(
    implementation = _corpus_manifest_impl,
    doc = "Writes <name>.tsv: one `display name<TAB>runfiles location` line " +
          "per source file, and carries the files as runfiles. Pass it to a " +
          "harness tool as -manifest=$(rlocationpath <target>).",
    attrs = {
        "srcs": attr.label_list(allow_files = True, mandatory = True),
    },
)
