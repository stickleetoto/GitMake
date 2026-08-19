package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Repo.Visibility != "private" {
		t.Fatalf("visibility=%q", c.Repo.Visibility)
	}
	if c.Git.Branch != "main" {
		t.Fatalf("branch=%q", c.Git.Branch)
	}
	if c.Source.StripRoot == nil || !*c.Source.StripRoot {
		t.Fatal("strip_root should default true")
	}
}

func TestRejectUnknownField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo","oops":true},"source":{"zip":"demo.zip"},"git":{}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestRejectUnsafeRepoName(t *testing.T) {
	c := Config{SchemaVersion: 1, Repo: RepoConfig{Name: "owner/repo", Visibility: "private"}, Source: SourceConfig{ZIP: "x.zip"}, Git: GitConfig{Branch: "main", InitialCommitMessage: "Initial", CommitMessage: "Update"}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid repo name")
	}
}

func TestRejectTrailingJSONValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{}} {}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestLoadUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{}}`)...)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("UTF-8 BOM should be accepted: %v", err)
	}
}

func TestRejectUTF16Config(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	if err := os.WriteFile(p, []byte{0xFF, 0xFE, '{', 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected UTF-16 rejection")
	}
}
