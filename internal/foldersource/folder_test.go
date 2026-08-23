package foldersource

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotHonorsIgnoreAndIsStable(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, data string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("README.md", "hello")
	mustWrite("src/main.py", "print('ok')")
	mustWrite("node_modules/x.js", "ignored")
	mustWrite("tmp/cache.bin", "ignored")
	mustWrite("keep.txt", "kept")
	mustWrite(".gitignore", "tmp/\n*.log\n")
	mustWrite("debug.log", "ignored")
	mustWrite("gitmake.json", "{}")

	dest := t.TempDir()
	r, err := Snapshot(root, dest)
	if err != nil {
		t.Fatal(err)
	}
	if r.Files != 4 {
		t.Fatalf("files=%d want 4", r.Files)
	}
	for _, rel := range []string{"README.md", "src/main.py", "keep.txt", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"node_modules/x.js", "tmp/cache.bin", "debug.log", "gitmake.json"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("unexpected copied %s", rel)
		}
	}
	h, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if h.SHA256 != r.SHA256 {
		t.Fatalf("hash mismatch snapshot=%s hash=%s", r.SHA256, h.SHA256)
	}

	mustWrite("node_modules/y.js", "ignored change")
	h2, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if h2.SHA256 != r.SHA256 {
		t.Fatalf("ignored file changed source hash")
	}
	mustWrite("src/main.py", "print('changed')")
	h3, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if h3.SHA256 == r.SHA256 {
		t.Fatalf("included file change did not change hash")
	}
}

func TestGitMakeIgnoreAndNegation(t *testing.T) {
	root := t.TempDir()
	for rel, data := range map[string]string{
		"a.txt": "a", "private/a.txt": "x", "private/keep.txt": "k",
		".gitmakeignore": "private/**\n!private/keep.txt\n",
	} {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(data), 0o644)
	}
	dest := t.TempDir()
	_, err := Snapshot(root, dest)
	if err != nil {
		t.Fatal(err)
	}
	// GitMake-specific negation can re-include an explicitly named file.
	if _, err := os.Stat(filepath.Join(dest, "private", "keep.txt")); err != nil {
		t.Fatalf("negated file should be included: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "private", "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("private/a.txt should stay ignored")
	}
}

func TestDetectProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DetectProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsProject || d.Score < 5 {
		t.Fatalf("detection=%+v", d)
	}
}

func BenchmarkHashThousandSmallFiles(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 1000; i++ {
		name := filepath.Join(root, "src", fmt.Sprintf("file_%04d.txt", i))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("gitmake benchmark payload\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Hash(root); err != nil {
			b.Fatal(err)
		}
	}
}
