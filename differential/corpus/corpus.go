// Package corpus loads the GALA source files the harness tools run over.
package corpus

import (
	"bufio"
	"cmp"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

// File is one corpus file.
type File struct {
	// Name identifies the file in reports: its path relative to the corpus
	// root, with forward slashes.
	Name string
	// Path is where the file was read from.
	Path string
	// Src is its contents.
	Src string
}

// Source says where to read a corpus from. Explicit files win over a
// directory (or a list of files), and a directory over a manifest, so a tool whose Bazel target
// passes the default -manifest can still be pointed at other input.
type Source struct {
	// Dir is a directory searched recursively for .gala files.
	Dir string
	// Manifest is a file written by the corpus_manifest Bazel rule.
	Manifest string
	// Samples is a file galop-sample wrote, by runfiles location or path:
	// generated sentences, named sample-0001 and on.
	Samples string
	// List is a file naming one input file per line.
	List string
	// Files are individual files.
	Files []string
}

// RegisterFlags adds -dir and -manifest to fs; the positional arguments,
// once parsed, go in Files.
func (s *Source) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&s.Dir, "dir", "", "read every .gala file under this directory")
	fs.StringVar(&s.Manifest, "manifest", "", "read the files a corpus_manifest lists, by runfiles location (the Bazel corpus)")
	fs.StringVar(&s.List, "list", "", "read the files this file names, one path per line")
	fs.StringVar(&s.Samples, "samples", "", "read the sentences a galop-sample file holds, by runfiles location or path")
}

// Load reads the corpus s names, sorted by Name.
func (s Source) Load() ([]File, error) {
	if s.List != "" && len(s.Files) == 0 {
		list, err := os.ReadFile(fromWorkingDir(s.List))
		if err != nil {
			return nil, err
		}
		s.Files = strings.Fields(string(list))
	}
	switch {
	case len(s.Files) > 0:
		files := make([]File, 0, len(s.Files))
		for _, p := range s.Files {
			p = fromWorkingDir(p)
			f, err := read(filepath.ToSlash(p), p)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
		return files, nil
	case s.Dir != "":
		return FromDir(fromWorkingDir(s.Dir))
	case s.Samples != "":
		return FromSamples(s.Samples)
	case s.Manifest != "":
		return FromManifest(s.Manifest)
	}
	return nil, fmt.Errorf("no input: pass -dir, -samples, -manifest or files")
}

// FromDir reads every .gala file under root.
func FromDir(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".gala" {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		f, err := read(filepath.ToSlash(rel), p)
		files = append(files, f)
		return err
	})
	if err == nil && len(files) == 0 {
		err = fmt.Errorf("%s: no .gala files", root)
	}
	return sorted(files), err
}

// FromManifest reads the files a corpus_manifest lists. The manifest and the
// files it names are given by their runfiles locations, so it works from a
// Bazel test or `bazel run`.
func FromManifest(manifest string) ([]File, error) {
	manifest, err := runfiles.Rlocation(manifest)
	if err != nil {
		return nil, err
	}
	m, err := os.Open(manifest)
	if err != nil {
		return nil, err
	}
	defer m.Close()
	var files []File
	lines := bufio.NewScanner(m)
	for lines.Scan() {
		name, rlocation, ok := strings.Cut(lines.Text(), "\t")
		if !ok {
			return nil, fmt.Errorf("%s: malformed line %q", manifest, lines.Text())
		}
		path, err := runfiles.Rlocation(rlocation)
		if err != nil {
			return nil, err
		}
		f, err := read(name, path)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}
	return sorted(files), nil
}

// Bytes is the total size of files.
func Bytes(files []File) int {
	n := 0
	for _, f := range files {
		n += len(f.Src)
	}
	return n
}

// Largest returns the n largest files, largest first.
func Largest(files []File, n int) []File {
	bySize := slices.Clone(files)
	slices.SortStableFunc(bySize, func(a, b File) int { return cmp.Compare(len(b.Src), len(a.Src)) })
	return bySize[:min(n, len(bySize))]
}

func read(name, path string) (File, error) {
	src, err := os.ReadFile(path)
	return File{Name: name, Path: path, Src: string(src)}, err
}

func sorted(files []File) []File {
	slices.SortFunc(files, func(a, b File) int { return cmp.Compare(a.Name, b.Name) })
	return files
}

// fromWorkingDir resolves a relative path against the directory `bazel run`
// was started from, which is not the binary's working directory.
func fromWorkingDir(p string) string {
	if wd := os.Getenv("BUILD_WORKING_DIRECTORY"); wd != "" && !filepath.IsAbs(p) {
		return filepath.Join(wd, p)
	}
	return p
}

// FromSamples reads the sentences galop-sample wrote to samples (a runfiles
// location, or else a path): each is its length in bytes on a line of its
// own, then the sentence and a newline. File i is named sample-<i>, from 1.
func FromSamples(samples string) ([]File, error) {
	path := fromWorkingDir(Locate(samples))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var files []File
	for rest := string(data); rest != ""; {
		head, body, ok := strings.Cut(rest, "\n")
		n, err := strconv.Atoi(head)
		if !ok || err != nil || n < 0 || len(body) < n+1 || body[n] != '\n' {
			return nil, fmt.Errorf("%s: malformed sample %d (a byte count line, the sentence, a newline)", samples, len(files)+1)
		}
		files = append(files, File{Name: fmt.Sprintf("sample-%04d", len(files)+1), Path: path, Src: body[:n]})
		rest = body[n+1:]
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no samples", samples)
	}
	return files, nil
}

// WriteSamples writes srcs to path in the format FromSamples reads.
func WriteSamples(path string, srcs []string) error {
	var sb strings.Builder
	for _, s := range srcs {
		fmt.Fprintf(&sb, "%d\n%s\n", len(s), s)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// Locate resolves a runfiles location to a path when it names one, and
// otherwise returns p as it is.
func Locate(p string) string {
	if loc, err := runfiles.Rlocation(p); err == nil {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}
	return p
}
