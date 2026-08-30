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

func Upgrade(currentVersion string, releases ReleaseClient) (string, bool, error) {
	tag, err := releases.LatestReleaseTag(releaseRepo)
	if err != nil {
		return "", false, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	cmp, err := compareReleaseVersions(latest, currentVersion)
	if err != nil {
		return "", false, fmt.Errorf("compare GitMake versions: %w", err)
	}
	if cmp <= 0 {
		return tag, false, nil
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return tag, false, fmt.Errorf("gitmake upgrade self-replacement is currently supported on Windows x64 only")
	}

	tmp, err := os.MkdirTemp("", "gitmake-upgrade-*")
	if err != nil {
		return "", false, err
	}
	// Do not remove the temp directory immediately: the Windows replacement
	// helper needs the extracted executable after this process exits.
	asset := fmt.Sprintf("GitMake_v%s_Windows_x64.zip", latest)
	zipPath, err := releases.DownloadReleaseAsset(releaseRepo, tag, asset, tmp)
	if err != nil {
		return "", false, err
	}
	checksumAsset := fmt.Sprintf("GitMake_v%s_SHA256.txt", latest)
	checksumPath, err := releases.DownloadReleaseAsset(releaseRepo, tag, checksumAsset, tmp)
	if err != nil {
		return "", false, fmt.Errorf("download upgrade checksum: %w", err)
	}
	if err := verifyChecksumFile(checksumPath, zipPath, asset); err != nil {
		return "", false, fmt.Errorf("verify upgrade package: %w", err)
	}
	newExe, err := extractExecutable(zipPath, tmp)
	if err != nil {
		return "", false, err
	}
	if err := StageReplacement(newExe); err != nil {
		return "", false, err
	}
	return tag, true, nil
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

func extractExecutable(zipPath, dst string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open downloaded release asset: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.EqualFold(filepath.Base(f.Name), "gitmake.exe") || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out := filepath.Join(dst, "gitmake-new.exe")
		w, err := os.Create(out)
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
	return "", fmt.Errorf("downloaded release asset does not contain gitmake.exe")
}
