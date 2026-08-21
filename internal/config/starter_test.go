package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureStarterNoZip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	created, err := EnsureStarter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected starter to be created")
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Repo.Name != "YOUR_REPOSITORY" || c.Source.ZIP != "YOUR_PROJECT.zip" {
		t.Fatalf("unexpected starter: repo=%q zip=%q", c.Repo.Name, c.Source.ZIP)
	}
}

func TestEnsureStarterDetectsSingleZip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ContextDiet_v0.1.0.zip"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "gitmake.json")
	created, err := EnsureStarter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected starter to be created")
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Repo.Name != "ContextDiet" {
		t.Fatalf("repo name = %q", c.Repo.Name)
	}
	if c.Source.ZIP != "ContextDiet_v0.1.0.zip" {
		t.Fatalf("zip = %q", c.Source.ZIP)
	}
}

func TestEnsureStarterDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	created, err := EnsureStarter(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("must not overwrite existing config")
	}
}

func TestResolveProjectZIPRepairsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	created, err := EnsureStarter(path)
	if err != nil || !created {
		t.Fatalf("EnsureStarter created=%v err=%v", created, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Demo_v1.2.3.zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	zipPath, repaired, err := ResolveProjectZIP(path, &c)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("expected config repair")
	}
	if filepath.Base(zipPath) != "Demo_v1.2.3.zip" || c.Source.ZIP != "Demo_v1.2.3.zip" || c.Repo.Name != "Demo" {
		t.Fatalf("unexpected repair: path=%q zip=%q repo=%q", zipPath, c.Source.ZIP, c.Repo.Name)
	}
	persisted, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Source.ZIP != "Demo_v1.2.3.zip" || persisted.Repo.Name != "Demo" {
		t.Fatalf("repair not persisted: %+v", persisted)
	}
}

func TestResolveProjectZIPRefusesStaleSingleZipForRealRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"old.zip"},"git":{}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_, repaired, err := ResolveProjectZIP(path, &c)
	if err == nil || repaired || !strings.Contains(err.Error(), "refusing to retarget") {
		t.Fatalf("expected safe retarget refusal: repaired=%v err=%v", repaired, err)
	}
}

func TestResolveProjectZIPMultipleCandidatesIsClearError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"missing.zip"},"git":{}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.zip", "b.zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveProjectZIP(path, &c)
	if err == nil || !strings.Contains(err.Error(), "a.zip") || !strings.Contains(err.Error(), "b.zip") {
		t.Fatalf("expected candidate list, got %v", err)
	}
}
