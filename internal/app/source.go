package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/archive"
	"gitmake/internal/config"
	"gitmake/internal/discovery"
	"gitmake/internal/foldersource"
)

type sourceSelection struct {
	Mode      string
	Path      string
	Repaired  bool
	Discovery *discovery.Report
	Folder    *foldersource.Detection
}

func explicitSource(cwd, arg string) (sourceSelection, error) {
	if strings.TrimSpace(arg) == "" {
		return sourceSelection{}, nil
	}
	p := arg
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p, _ = filepath.Abs(p)
	info, err := os.Stat(p)
	if err != nil {
		return sourceSelection{}, fmt.Errorf("source: %w", err)
	}
	if info.IsDir() {
		return sourceSelection{Mode: "folder", Path: p}, nil
	}
	if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(p), ".zip") {
		return sourceSelection{Mode: "zip", Path: p}, nil
	}
	return sourceSelection{}, fmt.Errorf("source must be a project folder or regular .zip file: %s", p)
}

func configuredSource(configPath string, cfg *config.Config, readOnly bool) (sourceSelection, error) {
	if cfg.Source.Folder != "" {
		p, err := config.ResolveProjectFolder(configPath, *cfg)
		if err != nil {
			return sourceSelection{}, err
		}
		return sourceSelection{Mode: "folder", Path: p}, nil
	}
	var p string
	var repaired bool
	var err error
	if readOnly {
		p, err = config.ResolveProjectZIPReadOnly(configPath, *cfg)
	} else {
		p, repaired, err = config.ResolveProjectZIP(configPath, cfg)
	}
	if err != nil {
		return sourceSelection{}, err
	}
	return sourceSelection{Mode: "zip", Path: p, Repaired: repaired}, nil
}

// inferSource prefers a clear project folder over adjacent archives. This lets
// `gitmake` work directly inside a source tree while preserving the old staging
// folder behavior where a folder only contains one or more ZIPs.
func inferSource(cwd string) (sourceSelection, error) {
	fd, err := foldersource.DetectProject(cwd)
	if err != nil {
		return sourceSelection{}, err
	}
	dr, err := discovery.Analyze(cwd)
	if err != nil {
		return sourceSelection{}, err
	}
	if fd.IsProject && (fd.Confidence == "high" || dr.SelectedSource == "") {
		return sourceSelection{Mode: "folder", Path: cwd, Folder: &fd, Discovery: &dr}, nil
	}
	if dr.SelectedSource != "" {
		return sourceSelection{Mode: "zip", Path: filepath.Join(cwd, dr.SelectedSource), Discovery: &dr, Folder: &fd}, nil
	}
	if fd.IsProject {
		return sourceSelection{Mode: "folder", Path: cwd, Folder: &fd, Discovery: &dr}, nil
	}
	if dr.NeedsInput {
		return sourceSelection{Discovery: &dr, Folder: &fd}, fmt.Errorf("multiple source candidates found; run `gitmake discover --json` or choose one explicitly with `gitmake Project.zip` or `gitmake .`")
	}
	return sourceSelection{Discovery: &dr, Folder: &fd}, fmt.Errorf("no project source found; run GitMake inside a project folder or provide a .zip file")
}

func hashSelectedSource(sel sourceSelection) (string, error) {
	if sel.Mode == "folder" {
		r, err := foldersource.Hash(sel.Path)
		if err != nil {
			return "", err
		}
		return r.SHA256, nil
	}
	return sha256File(sel.Path)
}

func snapshotSelectedSource(sel sourceSelection, cfg config.Config, dest string) (files, ignored int, hash string, err error) {
	if sel.Mode == "folder" {
		r, e := foldersource.Snapshot(sel.Path, dest)
		return r.Files, r.Ignored, r.SHA256, e
	}
	files, err = archive.ExtractSafe(sel.Path, dest, *cfg.Source.StripRoot)
	if err != nil {
		return 0, 0, "", err
	}
	hash, err = sha256File(sel.Path)
	return files, 0, hash, err
}

func configForSelection(configPath string, sel sourceSelection) (config.Config, error) {
	return config.ConfigForSource(configPath, sel.Path)
}

func createConfigForSelection(configPath string, sel sourceSelection) (config.Config, error) {
	return config.CreateForSource(configPath, sel.Path, false)
}

func sourceDisplay(sel sourceSelection) string {
	if sel.Mode == "folder" {
		return sel.Path
	}
	return filepath.Base(sel.Path)
}
