package upgrader

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyChecksumFile(t *testing.T) {
	d := t.TempDir()
	asset := "GitMake_v0.5.2_Windows_x64.zip"
	assetPath := filepath.Join(d, asset)
	data := []byte("package")
	if err := os.WriteFile(assetPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	check := filepath.Join(d, "GitMake_v0.5.2_SHA256.txt")
	if err := os.WriteFile(check, []byte(hex.EncodeToString(sum[:])+"  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksumFile(check, assetPath, asset); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksumFile(check, assetPath, asset); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
