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

func TestCompareReleaseVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.2", "1.2.1", 1},
		{"1.2.1", "1.2.1", 0},
		{"1.2.0", "1.2.1", -1},
		{"2.0.0", "1.99.99", 1},
		{"v1.10.0", "1.2.9", 1},
	}
	for _, tc := range cases {
		got, err := compareReleaseVersions(tc.a, tc.b)
		if err != nil {
			t.Fatalf("compare %s %s: %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Fatalf("compare %s %s: got %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if _, err := compareReleaseVersions("1.2-beta", "1.2.1"); err == nil {
		t.Fatal("expected invalid release version to be rejected")
	}
}

type fakeReleaseClient struct {
	tag       string
	downloads int
}

func (f *fakeReleaseClient) LatestReleaseTag(string) (string, error) { return f.tag, nil }
func (f *fakeReleaseClient) DownloadReleaseAsset(string, string, string, string) (string, error) {
	f.downloads++
	return "", nil
}

func TestUpgradeRefusesDowngradeBeforeDownload(t *testing.T) {
	f := &fakeReleaseClient{tag: "v1.2.1"}
	tag, staged, err := Upgrade("1.2.2", f)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v1.2.1" || staged {
		t.Fatalf("unexpected result tag=%q staged=%v", tag, staged)
	}
	if f.downloads != 0 {
		t.Fatalf("downgrade path downloaded %d assets", f.downloads)
	}
}
