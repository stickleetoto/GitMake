package upgrader

import (
	"archive/zip"
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
	newExe, err := extractExecutable(zipPath, tmp)
	if err != nil {
		return "", false, err
	}
	if err := StageReplacement(newExe); err != nil {
		return "", false, err
	}
	return tag, true, nil
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
