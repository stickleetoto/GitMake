package upgrader

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const releaseRepo = "stickleetoto/GitMake"

type ReleaseClient interface {
	LatestReleaseTag(target string) (string, error)
	DownloadReleaseAsset(target, tag, asset, dir string) (string, error)
}

// Outcome reports exactly what an upgrade attempt did, so the CLI never has to
// guess. Installed and Scheduled are mutually exclusive; when neither is set
// the current build was already up to date.
type Outcome struct {
	// Tag is the latest published release tag.
	Tag string
	// Installed reports a replacement that has already completed and been
	// verified on disk.
	Installed bool
	// Scheduled reports that the in-process replacement was impossible and a
	// helper will finish it after this process exits.
	Scheduled bool
	// Target is the executable path that was (or will be) replaced.
	Target string
	// PreviousImage is set when the displaced executable is still running and
	// therefore could not be deleted yet.
	PreviousImage string
	// HelperLog is the fallback helper's log path, set only when Scheduled.
	HelperLog string
}

func Upgrade(currentVersion string, releases ReleaseClient) (Outcome, error) {
	var out Outcome
	tag, err := releases.LatestReleaseTag(releaseRepo)
	if err != nil {
		return out, err
	}
	out.Tag = tag
	latest := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	cmp, err := compareReleaseVersions(latest, currentVersion)
	if err != nil {
		return out, fmt.Errorf("compare GitMake versions: %w", err)
	}
	if cmp <= 0 {
		return out, nil
	}

	assetName, exeName, err := platformAsset(latest)
	if err != nil {
		return out, err
	}
	target, err := resolveTarget()
	if err != nil {
		return out, err
	}
	out.Target = target

	tmp, err := os.MkdirTemp("", "gitmake-upgrade-*")
	if err != nil {
		return out, err
	}
	keepTmp := false
	defer func() {
		if !keepTmp {
			_ = os.RemoveAll(tmp)
		}
	}()

	zipPath, err := releases.DownloadReleaseAsset(releaseRepo, tag, assetName, tmp)
	if err != nil {
		return out, err
	}
	checksumAsset := fmt.Sprintf("GitMake_v%s_SHA256.txt", latest)
	checksumPath, err := releases.DownloadReleaseAsset(releaseRepo, tag, checksumAsset, tmp)
	if err != nil {
		return out, fmt.Errorf("download upgrade checksum: %w", err)
	}
	if err := verifyChecksumFile(checksumPath, zipPath, assetName); err != nil {
		return out, fmt.Errorf("verify upgrade package: %w", err)
	}
	newExe, err := extractExecutable(zipPath, tmp, exeName)
	if err != nil {
		return out, err
	}

	res, scheduled, helperLog, err := applyReplacement(newExe, target)
	if err != nil {
		return out, err
	}
	if scheduled {
		// The helper still needs the extracted executable after this process
		// exits, so the temp directory has to survive.
		keepTmp = true
		out.Scheduled = true
		out.HelperLog = helperLog
		return out, nil
	}
	out.Installed = true
	out.PreviousImage = res.Backup
	return out, nil
}

// platformAsset maps the running platform to its release asset and to the
// executable name packaged inside it.
func platformAsset(version string) (asset, exeName string, err error) {
	type key struct{ os, arch string }
	names := map[key]string{
		{"windows", "amd64"}: "Windows_x64",
		{"linux", "amd64"}:   "Linux_x64",
		{"linux", "arm64"}:   "Linux_arm64",
		{"darwin", "amd64"}:  "macOS_x64",
		{"darwin", "arm64"}:  "macOS_arm64",
	}
	suffix, ok := names[key{runtime.GOOS, runtime.GOARCH}]
	if !ok {
		return "", "", fmt.Errorf("gitmake upgrade does not publish a release build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	exeName = "gitmake"
	if runtime.GOOS == "windows" {
		exeName = "gitmake.exe"
	}
	return fmt.Sprintf("GitMake_v%s_%s.zip", version, suffix), exeName, nil
}

// resolveTarget is a seam so the real-process upgrade test can install into a
// scratch directory instead of replacing the running test binary.
var resolveTarget = replacementTarget

// replacementTarget is the executable this command will replace: the one the
// user actually invoked. Upgrading a copy run from Downloads updates that copy
// and leaves any installed copy alone, so the CLI reports the resolved path.
func replacementTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate current executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}

func compareReleaseVersions(a, b string) (int, error) {
	av, err := parseReleaseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("latest version %q: %w", a, err)
	}
	bv, err := parseReleaseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("current version %q: %w", b, err)
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1, nil
		}
		if av[i] > bv[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseReleaseVersion(v string) ([3]int, error) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("expected stable semantic version MAJOR.MINOR.PATCH")
	}
	for i, part := range parts {
		if part == "" {
			return out, fmt.Errorf("empty version component")
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid numeric component %q", part)
		}
		out[i] = n
	}
	return out, nil
}

func verifyChecksumFile(checksumPath, filePath, assetName string) error {
	f, err := os.Open(checksumPath)
	if err != nil {
		return err
	}
	defer f.Close()
	expected := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName || filepath.Base(name) == assetName {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s is missing", assetName)
	}
	if len(expected) != 64 {
		return fmt.Errorf("checksum for %s is not a SHA-256 digest", assetName)
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return fmt.Errorf("checksum for %s is invalid: %w", assetName, err)
	}
	actual, err := sha256File(filePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", assetName, expected, actual)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractExecutable(zipPath, dst, exeName string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open downloaded release asset: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.EqualFold(filepath.Base(f.Name), exeName) || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out := filepath.Join(dst, "gitmake-new"+filepath.Ext(exeName))
		w, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(w, rc)
		closeReadErr := rc.Close()
		closeWriteErr := w.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeReadErr != nil {
			return "", closeReadErr
		}
		if closeWriteErr != nil {
			return "", closeWriteErr
		}
		return out, nil
	}
	return "", fmt.Errorf("downloaded release asset does not contain %s", exeName)
}
