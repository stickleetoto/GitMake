package projectid

import (
	"strings"
	"testing"
)

func TestWriteReadValidate(t *testing.T) {
	dir := t.TempDir()
	written, err := Write(dir, "Owner/Repo")
	if err != nil {
		t.Fatal(err)
	}
	if written.ProjectID == "" {
		t.Fatal("missing project id")
	}
	got, exists, err := Validate(dir, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got.Repository != "Owner/Repo" {
		t.Fatalf("got %#v exists=%v", got, exists)
	}
}

func TestValidateMismatch(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "Owner/RepoA"); err != nil {
		t.Fatal(err)
	}
	_, _, err := Validate(dir, "Owner/RepoB")
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("err=%v", err)
	}
}
