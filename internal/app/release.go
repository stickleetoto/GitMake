package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/config"
	"gitmake/internal/github"
	"gitmake/internal/gitops"
)

type releasePlan struct {
	enabled      bool
	disabledWhy  string
	skipExisting bool
	existingURL  string
	spec         github.ReleaseCreateOptions
}

func prepareReleasePlan(configPath, target string, repoExists bool, cfg config.Config, noRelease bool, git gitops.Client, gh github.Client) (releasePlan, error) {
	if !cfg.Release.Enabled {
		return releasePlan{disabledWhy: "Release disabled"}, nil
	}
	if noRelease {
		return releasePlan{disabledWhy: "Release skipped (--no-release)"}, nil
	}
	if err := git.ValidateTag(cfg.Release.Tag); err != nil {
		return releasePlan{}, err
	}

	if repoExists {
		info, exists, err := gh.Release(target, cfg.Release.Tag)
		if err != nil {
			return releasePlan{}, err
		}
		if exists {
			if cfg.Release.OnExisting == "skip" {
				return releasePlan{enabled: true, skipExisting: true, existingURL: info.URL, spec: github.ReleaseCreateOptions{Tag: cfg.Release.Tag}}, nil
			}
			return releasePlan{}, fmt.Errorf("release %s already exists in %s (set release.on_existing to \"skip\" to ignore it)", cfg.Release.Tag, target)
		}
	}

	base := filepath.Dir(configPath)
	notesFile := ""
	if cfg.Release.NotesFile != "" {
		p, err := resolveRegularFile(base, cfg.Release.NotesFile, "release.notes_file")
		if err != nil {
			return releasePlan{}, err
		}
		notesFile = p
	}

	assets := make([]string, 0, len(cfg.Release.Assets))
	seenNames := make(map[string]string)
	for _, configured := range cfg.Release.Assets {
		p, err := resolveRegularFile(base, configured, "release asset")
		if err != nil {
			return releasePlan{}, err
		}
		if strings.Contains(p, "#") {
			return releasePlan{}, fmt.Errorf("release asset path contains '#', which GitHub CLI treats as an asset label separator: %s", p)
		}
		nameKey := strings.ToLower(filepath.Base(p))
		if previous, ok := seenNames[nameKey]; ok {
			return releasePlan{}, fmt.Errorf("release assets have duplicate file names: %s and %s", previous, p)
		}
		seenNames[nameKey] = p
		assets = append(assets, p)
	}

	generateNotes := false
	if cfg.Release.GenerateNotes != nil {
		generateNotes = *cfg.Release.GenerateNotes
	}

	return releasePlan{
		enabled: true,
		spec: github.ReleaseCreateOptions{
			Tag:           cfg.Release.Tag,
			Title:         cfg.Release.Title,
			Notes:         cfg.Release.Notes,
			NotesFile:     notesFile,
			GenerateNotes: generateNotes,
			Assets:        assets,
			Draft:         cfg.Release.Draft,
			Prerelease:    cfg.Release.Prerelease,
			Latest:        cfg.Release.Latest,
		},
	}, nil
}

func resolveRegularFile(base, configured, field string) (string, error) {
	p := configured
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s not found: %s", field, p)
		}
		return "", fmt.Errorf("check %s: %w", field, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular file: %s", field, p)
	}
	return p, nil
}

func finishRelease(plan releasePlan, target, branch string, dryRun bool, gh github.Client) error {
	if !plan.enabled {
		fmt.Printf("[8/8] %s\n", plan.disabledWhy)
		return nil
	}
	if plan.skipExisting {
		fmt.Printf("[8/8] Release %s already exists — skipped\n", plan.spec.Tag)
		if plan.existingURL != "" {
			fmt.Println(plan.existingURL)
		}
		return nil
	}

	plan.spec.Target = branch
	if dryRun {
		fmt.Printf("[8/8] Dry run — release %s will NOT be created\n", plan.spec.Tag)
		if len(plan.spec.Assets) > 0 {
			fmt.Println("Release assets:")
			for _, asset := range plan.spec.Assets {
				fmt.Println("  -", filepath.Base(asset))
			}
		}
		return nil
	}

	fmt.Printf("[8/8] Creating GitHub release %s\n", plan.spec.Tag)
	url, err := gh.CreateRelease(target, plan.spec)
	if err != nil {
		return err
	}
	fmt.Println("✓ Released:", plan.spec.Tag)
	if url != "" {
		fmt.Println(url)
	}
	return nil
}
