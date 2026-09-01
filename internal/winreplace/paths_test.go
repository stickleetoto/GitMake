package winreplace

import (
	"os"
	"path/filepath"
	"testing"
)

// awkwardDir is the kind of install path real users have: a space and
// non-ASCII characters. Korean Windows installs are the reported case.
const awkwardDir = "내 프로그램 폴더"

func TestReplaceExecutableHandlesSpacesAndNonASCIIPaths(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, awkwardDir, "Program Files", "gitmake"+exeExt())

	source := filepath.Join(root, "staged executable")
	if err := os.WriteFile(source, []byte("new-image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-image"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := ReplaceExecutable(source, target)
	if err != nil {
		t.Fatalf("replace at %q: %v", target, err)
	}
	if !res.Replaced {
		t.Fatal("expected an existing executable to be replaced")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-image" {
		t.Fatalf("target content = %q", string(got))
	}
	// Nothing was running the old image, so it must already be gone rather
	// than left as clutter in the install directory.
	if res.Backup != "" {
		t.Fatalf("displaced image was not cleaned up: %s", res.Backup)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("install directory holds leftover files: %v", names)
	}
}
