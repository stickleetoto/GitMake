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
	"strings"

	"gitmake/internal/github"
)

const releaseRepo = "stickleetoto/GitMake"

func Upgrade(currentVersion string, gh github.Client) (string, bool, error) {
	tag, err := gh.LatestReleaseTag(releaseRepo)
	if err != nil {
		return "", false, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if latest == currentVersion {
		return tag, false, nil
	}
	tmp, err := os.MkdirTemp("", "gitmake-upgrade-*")
	if err != nil {
		return "", false, err
	}
	// Do not remove the temp directory immediately: the Windows replacement
	// helper needs the extracted executable after this process exits.
	asset := fmt.Sprintf("GitMake_v%s_Windows_x64.zip", latest)
	zipPath, err := gh.DownloadReleaseAsset(releaseRepo, tag, asset, tmp)
	if err != nil {
		return "", false, err
	}
	checksumAsset := fmt.Sprintf("GitMake_v%s_SHA256.txt", latest)
	checksumPath, err := gh.DownloadReleaseAsset(releaseRepo, tag, checksumAsset, tmp)
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
		defer rc.Close()
		out := filepath.Join(dst, "gitmake-new.exe")
		w, err := os.Create(out)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(w, rc)
		closeErr := w.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return out, nil
	}
	return "", fmt.Errorf("downloaded release asset does not contain gitmake.exe")
}
