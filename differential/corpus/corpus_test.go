package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSamplesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.txt")
	srcs := []string{"a b", "", "line\nbreak", "héllo 12"}
	if err := WriteSamples(path, srcs); err != nil {
		t.Fatal(err)
	}
	files, err := FromSamples(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(srcs) {
		t.Fatalf("read %d samples, want %d", len(files), len(srcs))
	}
	for i, f := range files {
		if f.Src != srcs[i] {
			t.Errorf("sample %d = %q, want %q", i, f.Src, srcs[i])
		}
	}
}

func TestSamplesRejectBadFraming(t *testing.T) {
	for name, text := range map[string]string{
		"header with trailing text": "3x\nabc\n",
		"negative length":           "-1\nabc\n",
		"no newline after sample":   "3\nabcd\n",
		"length past the end":       "9\nabc\n",
		"no header newline":         "3",
		"empty file":                "",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.txt")
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			if files, err := FromSamples(path); err == nil {
				t.Errorf("read %d samples, want an error", len(files))
			}
		})
	}
}
