package discovery

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Archive struct {
	Name           string   `json:"name"`
	SourceScore    int      `json:"source_score"`
	AssetScore     int      `json:"asset_score"`
	Classification string   `json:"classification"`
	Reasons        []string `json:"reasons,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type Report struct {
	Schema                string    `json:"schema"`
	Directory             string    `json:"directory"`
	Archives              []Archive `json:"archives"`
	SelectedSource        string    `json:"selected_source,omitempty"`
	SourceConfidence      string    `json:"source_confidence,omitempty"`
	SourceConfidenceScore float64   `json:"source_confidence_score,omitempty"`
	SelectedSourceScore   int       `json:"selected_source_score,omitempty"`
	SelectedEvidence      []string  `json:"selected_evidence,omitempty"`
	ReleaseAssets         []string  `json:"release_assets,omitempty"`
	Unknown               []string  `json:"unknown,omitempty"`
	NeedsInput            bool      `json:"needs_input"`
	Reason                string    `json:"reason,omitempty"`
}

func Analyze(dir string) (Report, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Report{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Report{}, fmt.Errorf("scan directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	r := Report{Schema: "gitmake.discovery/v1", Directory: abs}
	for _, name := range names {
		a := inspect(filepath.Join(abs, name), name)
		r.Archives = append(r.Archives, a)
	}
	resolve(&r)
	return r, nil
}

func inspect(path, name string) Archive {
	a := Archive{Name: name}
	lower := strings.ToLower(name)
	add := func(src, asset int, reason string) {
		a.SourceScore += src
		a.AssetScore += asset
		if reason != "" {
			a.Reasons = append(a.Reasons, reason)
		}
	}

	// Filename is only a supporting signal. ZIP contents carry more weight.
	for _, token := range []string{"source", "src", "源码", "소스"} {
		if strings.Contains(lower, token) {
			add(3, 0, "filename suggests source archive")
			break
		}
	}
	for _, token := range []string{"windows", "win64", "win32", "linux", "darwin", "macos", "osx", "x64", "amd64", "arm64", "aarch64", "portable", "installer", "setup", "binary", "binaries"} {
		if strings.Contains(lower, token) {
			add(-1, 4, "filename suggests release artifact")
			break
		}
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		a.Error = fmt.Sprintf("open ZIP: %v", err)
		a.Classification = "invalid"
		return a
	}
	defer zr.Close()

	markerSeen := map[string]bool{}
	sourceDir := false
	binarySeen := false
	docsOnly := true
	regular := 0
	for _, f := range zr.File {
		n := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		if n == "" || strings.HasSuffix(n, "/") {
			continue
		}
		regular++
		base := strings.ToLower(filepath.Base(n))
		parts := strings.Split(strings.ToLower(n), "/")
		if len(parts) > 1 {
			first := parts[0]
			if first == "src" || first == "source" || first == "cmd" || first == "internal" || first == "lib" || first == "app" || first == "packages" {
				sourceDir = true
			}
		}
		switch base {
		case "go.mod", "cargo.toml", "package.json", "pyproject.toml", "setup.py", "requirements.txt", "pom.xml", "build.gradle", "build.gradle.kts", "cmakelists.txt", "makefile", "composer.json", "gemfile":
			markerSeen[base] = true
		}
		ext := strings.ToLower(filepath.Ext(base))
		switch ext {
		case ".go", ".rs", ".c", ".h", ".cpp", ".hpp", ".py", ".js", ".ts", ".tsx", ".jsx", ".java", ".kt", ".cs", ".swift", ".rb", ".php":
			docsOnly = false
			a.SourceScore++
		case ".exe", ".msi", ".dll", ".so", ".dylib", ".appimage", ".deb", ".rpm", ".apk":
			binarySeen = true
			docsOnly = false
		default:
			if base != "readme.md" && base != "license" && base != "license.md" && base != "changelog.md" && base != "quickstart.txt" {
				docsOnly = false
			}
		}
	}

	if len(markerSeen) > 0 {
		a.SourceScore += 6
		a.Reasons = append(a.Reasons, "contains project manifest/build marker")
	}
	if sourceDir {
		a.SourceScore += 4
		a.Reasons = append(a.Reasons, "contains source-oriented directory structure")
	}
	if binarySeen {
		a.AssetScore += 6
		a.SourceScore -= 2
		a.Reasons = append(a.Reasons, "contains packaged binary artifacts")
	}
	if regular == 0 {
		a.SourceScore -= 4
		a.AssetScore -= 2
		a.Reasons = append(a.Reasons, "contains no regular files")
	}
	if docsOnly && regular > 0 {
		a.AssetScore++
		a.Reasons = append(a.Reasons, "contains documentation-only payload")
	}

	switch {
	case a.SourceScore >= 6 && a.SourceScore >= a.AssetScore+2:
		a.Classification = "source"
	case a.AssetScore >= 4 && a.AssetScore > a.SourceScore:
		a.Classification = "release_asset"
	default:
		a.Classification = "unknown"
	}
	return a
}

func sourceConfidenceScore(a Archive) float64 {
	margin := a.SourceScore - a.AssetScore
	score := 0.55 + float64(a.SourceScore)*0.025 + float64(margin)*0.02
	if score < 0.50 {
		score = 0.50
	}
	if score > 0.99 {
		score = 0.99
	}
	return score
}

func selectSource(r *Report, a Archive, confidence string) {
	r.SelectedSource = a.Name
	r.SourceConfidence = confidence
	r.SourceConfidenceScore = sourceConfidenceScore(a)
	r.SelectedSourceScore = a.SourceScore
	r.SelectedEvidence = append([]string(nil), a.Reasons...)
}

func resolve(r *Report) {
	valid := make([]Archive, 0, len(r.Archives))
	var sources []Archive
	for _, a := range r.Archives {
		if a.Error == "" {
			valid = append(valid, a)
		}
		switch a.Classification {
		case "source":
			sources = append(sources, a)
		case "release_asset":
			r.ReleaseAssets = append(r.ReleaseAssets, a.Name)
		case "unknown", "invalid":
			r.Unknown = append(r.Unknown, a.Name)
		}
	}
	if len(valid) == 0 {
		if len(r.Archives) == 0 {
			r.Reason = "no_zip_files"
		} else {
			r.Reason = "no_valid_zip_files"
		}
		return
	}
	if len(valid) == 1 {
		if valid[0].Classification == "release_asset" {
			r.NeedsInput = true
			r.Reason = "single_archive_looks_like_release_asset"
			return
		}
		confidence := "single_archive"
		if valid[0].Classification == "unknown" {
			confidence = "single_archive_low"
		}
		selectSource(r, valid[0], confidence)
		return
	}

	// Once two archives independently look like real source projects, do not
	// use filename hints to break the tie. Asking is safer than publishing the
	// wrong project just because one happened to contain "Source" in its name.
	if len(sources) > 1 {
		r.NeedsInput = true
		r.Reason = "multiple_source_candidates"
		return
	}
	if len(sources) == 1 {
		chosen := sources[0]
		// An unknown archive with nearly the same source evidence is treated as
		// ambiguous. This prevents a filename hint from silently choosing the
		// wrong project when two plausible source bundles sit together.
		for _, candidate := range valid {
			if candidate.Name == chosen.Name || candidate.Classification == "release_asset" {
				continue
			}
			if candidate.SourceScore >= chosen.SourceScore-2 {
				r.NeedsInput = true
				r.Reason = "source_confidence_margin_too_small"
				return
			}
		}
		confidence := "medium"
		if chosen.SourceScore >= 10 && chosen.SourceScore-chosen.AssetScore >= 6 {
			confidence = "high"
		}
		selectSource(r, chosen, confidence)
		return
	}

	// Multiple archives exist but none has enough source evidence. Refuse to
	// guess; the user/agent can resolve this with an explicit ZIP argument.
	r.NeedsInput = true
	r.Reason = "multiple_source_candidates"
}
