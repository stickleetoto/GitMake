package upgrader

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	out, err := Upgrade("1.2.2", f)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tag != "v1.2.1" || out.Installed || out.Scheduled {
		t.Fatalf("unexpected outcome %+v", out)
	}
	if f.downloads != 0 {
		t.Fatalf("downgrade path downloaded %d assets", f.downloads)
	}
}

func TestPlatformAssetMatchesPublishedReleaseNames(t *testing.T) {
	asset, exeName, err := platformAsset("1.2.6")
	if err != nil {
		t.Skipf("no published build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if !strings.HasPrefix(asset, "GitMake_v1.2.6_") || !strings.HasSuffix(asset, ".zip") {
		t.Fatalf("unexpected asset name %q", asset)
	}
	wantExe := "gitmake"
	if runtime.GOOS == "windows" {
		wantExe = "gitmake.exe"
	}
	if exeName != wantExe {
		t.Fatalf("packaged executable name = %q, want %q", exeName, wantExe)
	}
}

// --- real-package upgrade lifecycle -----------------------------------------

// releaseFixture serves assets from a directory the test built, exercising the
// same download → checksum → extract → replace pipeline the real updater runs.
type releaseFixture struct {
	tag string
	dir string
}

func (r releaseFixture) LatestReleaseTag(string) (string, error) { return r.tag, nil }

func (r releaseFixture) DownloadReleaseAsset(_, _, asset, dir string) (string, error) {
	src, err := os.Open(filepath.Join(r.dir, asset))
	if err != nil {
		return "", err
	}
	defer src.Close()
	path := filepath.Join(dir, asset)
	dst, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return path, nil
}

func buildVersionPrinter(t *testing.T, dir, version string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("real-package upgrade test skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	src := t.TempDir()
	main := strings.Replace(`package main

import "fmt"

func main() { fmt.Print("__VERSION__") }
`, "__VERSION__", version, 1)
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module probe\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "probe-"+version)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = src
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build probe executable: %v: %s", err, string(combined))
	}
	return out
}

// buildReleaseFixture packages exePath the way a real GitMake release does and
// writes a matching SHA256 manifest.
func buildReleaseFixture(t *testing.T, dir, version, exePath string, corruptChecksum bool) releaseFixture {
	t.Helper()
	assetName, exeName, err := platformAsset(version)
	if err != nil {
		t.Skip(err.Error())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(dir, assetName)
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	// Include a sibling file so extraction has to select the executable.
	if w, err := zw.Create("README.md"); err == nil {
		_, _ = w.Write([]byte("release notes"))
	}
	w, err := zw.Create(exeName)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	digest, err := sha256File(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if corruptChecksum {
		digest = strings.Repeat("a", 64)
	}
	manifest := digest + "  " + assetName + "\n"
	if err := os.WriteFile(filepath.Join(dir, "GitMake_v"+version+"_SHA256.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return releaseFixture{tag: "v" + version, dir: dir}
}

func useTarget(t *testing.T, path string) {
	t.Helper()
	previous := resolveTarget
	resolveTarget = func() (string, error) { return path, nil }
	t.Cleanup(func() { resolveTarget = previous })
}

func tempUpgradeDirs(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "gitmake-upgrade-*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// TestUpgradeInstallsTheNewExecutable is the acceptance criterion from the
// updater contract: after upgrade, a fresh invocation of the target path must
// report the new version. Nothing below is mocked past the release HTTP layer.
func TestUpgradeInstallsTheNewExecutable(t *testing.T) {
	root := t.TempDir()
	oldExe := buildVersionPrinter(t, filepath.Join(root, "old"), "1.2.5")
	newExe := buildVersionPrinter(t, filepath.Join(root, "new"), "1.2.6")

	target := filepath.Join(root, "Programs", "GitMake", "gitmake")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(oldExe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, installed, 0o755); err != nil {
		t.Fatal(err)
	}
	useTarget(t, target)

	before := tempUpgradeDirs(t)
	fixture := buildReleaseFixture(t, filepath.Join(root, "release"), "1.2.6", newExe, false)

	out, err := Upgrade("1.2.5", fixture)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if !out.Installed {
		t.Fatalf("upgrade did not report a completed install: %+v", out)
	}
	if out.Scheduled {
		t.Fatal("in-process replacement must not report a scheduled replacement")
	}
	if out.Target != target {
		t.Fatalf("replaced %q, want %q", out.Target, target)
	}

	got, err := exec.Command(target).Output()
	if err != nil {
		t.Fatalf("run upgraded executable: %v", err)
	}
	if strings.TrimSpace(string(got)) != "1.2.6" {
		t.Fatalf("upgraded executable reports %q, want 1.2.6", strings.TrimSpace(string(got)))
	}

	if after := tempUpgradeDirs(t); len(after) != len(before) {
		t.Fatalf("upgrade left %d temporary download directories behind", len(after)-len(before))
	}
}

// TestUpgradeFailsClosedOnChecksumMismatch proves a corrupted or substituted
// package can never reach the install target.
func TestUpgradeFailsClosedOnChecksumMismatch(t *testing.T) {
	root := t.TempDir()
	newExe := buildVersionPrinter(t, filepath.Join(root, "new"), "1.2.6")

	target := filepath.Join(root, "Programs", "GitMake", "gitmake")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}
	useTarget(t, target)

	fixture := buildReleaseFixture(t, filepath.Join(root, "release"), "1.2.6", newExe, true)
	out, err := Upgrade("1.2.5", fixture)
	if err == nil {
		t.Fatal("expected a checksum mismatch to abort the upgrade")
	}
	if !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("unexpected error %v", err)
	}
	if out.Installed || out.Scheduled {
		t.Fatalf("failed upgrade reported progress: %+v", out)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatalf("install target was modified by a rejected package: %q %v", string(got), err)
	}
}

// TestUpgradeReportsMissingAssetWithoutTouchingTarget covers a release that is
// published before its platform asset finishes uploading.
func TestUpgradeReportsMissingAssetWithoutTouchingTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "gitmake")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.WriteFile(target, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}
	useTarget(t, target)

	empty := releaseFixture{tag: "v1.2.6", dir: filepath.Join(root, "empty")}
	if err := os.MkdirAll(empty.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Upgrade("1.2.5", empty); err == nil {
		t.Fatal("expected a missing release asset to fail the upgrade")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatalf("install target changed after a failed download: %q %v", string(got), err)
	}
}
