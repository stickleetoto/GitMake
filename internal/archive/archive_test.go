package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractSafeStripRoot(t *testing.T) {
	p := makeZip(t, map[string]string{"project/a.txt": "a", "project/sub/b.txt": "b"})
	dest := t.TempDir()
	n, err := ExtractSafe(p, dest, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("written=%d", n)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestRejectTraversal(t *testing.T) {
	p := makeZip(t, map[string]string{"../evil.txt": "x"})
	if _, err := ExtractSafe(p, t.TempDir(), false); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestRejectDotGit(t *testing.T) {
	p := makeZip(t, map[string]string{"project/.git/config": "x"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected .git rejection")
	}
}

func TestNoStripForRootFiles(t *testing.T) {
	p := makeZip(t, map[string]string{"README.md": "x", "src/a.txt": "a"})
	dest := t.TempDir()
	if _, err := ExtractSafe(p, dest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRejectWindowsReservedName(t *testing.T) {
	p := makeZip(t, map[string]string{"project/CON.txt": "x"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected Windows reserved-name rejection")
	}
}

func TestRejectWindowsTrailingDot(t *testing.T) {
	p := makeZip(t, map[string]string{"project/bad./x.txt": "x"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected trailing-dot rejection")
	}
}

func TestRejectCaseCollision(t *testing.T) {
	p := makeZip(t, map[string]string{"project/A.txt": "a", "project/a.txt": "b"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected Windows case-collision rejection")
	}
}

func TestRejectFileDirectoryConflict(t *testing.T) {
	p := makeZip(t, map[string]string{"project/a": "file", "project/a/b.txt": "child"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected file/directory conflict rejection")
	}
}

func TestRejectEmbeddedDotDot(t *testing.T) {
	p := makeZip(t, map[string]string{"project/a/../b.txt": "x"})
	if _, err := ExtractSafe(p, t.TempDir(), true); err == nil {
		t.Fatal("expected embedded .. rejection")
	}
}
