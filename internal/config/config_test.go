package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestReleaseDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0"}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Release.GenerateNotes == nil || !*c.Release.GenerateNotes {
		t.Fatal("release.generate_notes should default true when no notes are supplied")
	}
	if c.Release.OnExisting != "error" {
		t.Fatalf("release.on_existing=%q", c.Release.OnExisting)
	}
}

func TestReleaseExplicitNotesDisableGeneratedNotesByDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","notes":"hello"}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Release.GenerateNotes == nil || *c.Release.GenerateNotes {
		t.Fatal("release.generate_notes should default false when explicit notes are supplied")
	}
}

func TestReleaseRequiresTag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{},"release":{"enabled":true}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected missing release.tag to fail")
	}
}

func TestReleaseRejectsNotesAndNotesFileTogether(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","notes":"x","notes_file":"notes.md"}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected release.notes + release.notes_file to fail")
	}
}

func TestReleaseRejectsNoNotesWhenGenerationDisabled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gitmake.json")
	data := `{"repo":{"name":"demo"},"source":{"zip":"demo.zip"},"git":{},"release":{"enabled":true,"tag":"v1.0.0","generate_notes":false}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected non-interactive release validation to fail")
	}
}

func TestResolveProjectZIPReadOnlyDoesNotRepairConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "gitmake.json")
	strip := true
	cfg := Config{
		SchemaVersion: CurrentSchemaVersion,
		Repo:          RepoConfig{Name: "Demo", Visibility: "private"},
		Source:        SourceConfig{ZIP: "missing.zip", StripRoot: &strip},
		Git:           GitConfig{Branch: "main", InitialCommitMessage: "Initial commit", CommitMessage: "Update repository"},
	}
	if err := Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "actual.zip"), []byte("not important"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(configPath)
	if _, err := ResolveProjectZIPReadOnly(configPath, cfg); err == nil || !strings.Contains(err.Error(), "will not repair") {
		t.Fatalf("expected read-only repair refusal, got %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if string(before) != string(after) {
		t.Fatal("read-only resolver modified config")
	}
}
