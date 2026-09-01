package securityscan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretPathAndContentBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "code.txt"), []byte("github_"+"pat_"+"abcdefghijklmnopqrstuvwxyz123456"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: true, WarnFileBytes: 50, MaxGitFileBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Blocking || len(r.Findings) < 2 {
		t.Fatalf("expected blocking secret findings, got %#v", r)
	}
}

func TestAllowSecretPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixtures.pem"), []byte("-----BEGIN "+"PRIVATE KEY-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: true, AllowSecretPaths: []string{"fixtures.pem"}, WarnFileBytes: 1024, MaxGitFileBytes: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("allow path should suppress findings: %#v", r.Findings)
	}
}

func TestLargeFileMarkedLFSIsNotBlocking(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model.bin"), make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: false, WarnFileBytes: 64, MaxGitFileBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.LargeFiles) != 1 || !r.LargeFiles[0].LFSMarked || r.LargeFiles[0].Blocking {
		t.Fatalf("unexpected large file result: %#v", r.LargeFiles)
	}
	if !r.LFSRequired {
		t.Fatal("expected LFSRequired")
	}
}

func TestScannerTestFixturesDoNotContainLiteralSecretSignatures(t *testing.T) {
	data, err := os.ReadFile("scan_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range contentRules {
		if rule.re.Match(data) {
			t.Fatalf("security scanner test source contains a literal %s signature and would block GitMake self-publish", rule.kind)
		}
	}
}

func TestRepositorySourceTreeDoesNotSelfTriggerSecretScan(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	r, err := Scan(root, Options{SecretScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("GitMake source tree must pass its own secret scan before self-publish: %#v", r.Findings)
	}
}

func TestSecretScanReportsAllKindsAcrossAndWithinFiles(t *testing.T) {
	root := t.TempDir()
	aws := "AKIA" + "ABCDEFGHIJKLMNOP"
	slack := "xox" + "b-1234567890-abcdefghijklmnopqrstuv"
	github := "gh" + "p_abcdefghijklmnopqrstuvwxyz012345"
	if err := os.WriteFile(filepath.Join(root, "leak_one.txt"), []byte(aws+"\n"+github+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "leak_two.txt"), []byte(slack+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Blocking || r.ScannedFiles != 2 {
		t.Fatalf("unexpected scan summary: %#v", r)
	}
	got := map[string]bool{}
	for _, f := range r.Findings {
		got[f.Path+":"+f.Kind] = true
	}
	for _, want := range []string{
		"leak_one.txt:aws_access_key",
		"leak_one.txt:github_token",
		"leak_two.txt:slack_token",
	} {
		if !got[want] {
			t.Fatalf("missing finding %s; got %#v", want, r.Findings)
		}
	}
	if len(r.Findings) != 3 {
		t.Fatalf("expected exactly three distinct secret kinds, got %#v", r.Findings)
	}
}
