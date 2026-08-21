package discovery

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, dir, name string, files map[string]string) {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, body := range files {
		w, err := zw.Create(n)
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
}

func TestAnalyzeSelectsSourceAmongReleaseAssets(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "Demo_v1_Source.zip", map[string]string{"go.mod": "module demo\n", "cmd/demo/main.go": "package main\n"})
	writeZip(t, dir, "Demo_v1_Windows_x64.zip", map[string]string{"demo.exe": "binary", "README.md": "x"})
	writeZip(t, dir, "Demo_v1_Linux_x64.zip", map[string]string{"demo": "binary", "README.md": "x"})
	r, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.SelectedSource != "Demo_v1_Source.zip" {
		t.Fatalf("selected %q", r.SelectedSource)
	}
	if r.NeedsInput {
		t.Fatal("unexpected ambiguity")
	}
	if len(r.ReleaseAssets) != 2 {
		t.Fatalf("assets=%v", r.ReleaseAssets)
	}
}

func TestAnalyzeRefusesTwoSourceProjects(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "Alpha.zip", map[string]string{"go.mod": "module a\n", "src/main.go": "package main\n"})
	writeZip(t, dir, "Beta.zip", map[string]string{"package.json": "{}", "src/index.js": "x"})
	r, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.NeedsInput || r.SelectedSource != "" {
		t.Fatalf("report=%+v", r)
	}
}

func TestAnalyzeSingleArchiveAlwaysSelects(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "payload.zip", map[string]string{"hello.txt": "hi"})
	r, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.SelectedSource != "payload.zip" {
		t.Fatalf("selected %q", r.SelectedSource)
	}
}
