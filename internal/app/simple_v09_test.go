package app

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitmake/internal/config"
	"gitmake/internal/projectid"
)

func TestInferSourceFolderAndStrongZIPIsAmbiguous(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.test/live\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "src", "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(d, "Other_Source.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{"pyproject.toml": "[project]\nname='other'\n", "src/main.py": "print('x')\n", "README.md": "# Other\n"} {
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

	_, err = inferSource(d)
	var amb *sourceAmbiguityError
	if !errors.As(err, &amb) {
		t.Fatalf("expected sourceAmbiguityError, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %+v", amb.Candidates)
	}
	if amb.Candidates[0].Mode != "folder" || !amb.Candidates[0].Recommended {
		t.Fatalf("folder candidate should be recommended: %+v", amb.Candidates)
	}
}

func TestFolderProjectMemoryOverridesInferredRepo(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "README.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := projectid.Write(d, "owner/original-repo"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ConfigForFolder(filepath.Join(d, "gitmake.json"), d)
	if err != nil {
		t.Fatal(err)
	}
	used, err := applyFolderProjectMemory(sourceSelection{Mode: "folder", Path: d}, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("expected project memory to be used")
	}
	if cfg.Repo.Owner != "owner" || cfg.Repo.Name != "original-repo" {
		t.Fatalf("unexpected target: %+v", cfg.Repo)
	}
}
