package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gitmake/internal/planstore"
)

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

func classifyMachineError(err error, state *PipelineState) *MachineError {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	out := &MachineError{Kind: "runtime_error", Code: "RUNTIME_ERROR", Message: msg}
	if state != nil {
		out.Stage = state.Stage
	}
	switch {
	case strings.Contains(lower, "potential secrets detected"):
		out.Code = "SECRET_DETECTED"
		out.Recoverable = true
		out.SuggestedAction = "Remove the secret from the selected source or explicitly allow a safe fixture path in security.allow_secret_paths."
	case strings.Contains(lower, "large file") && strings.Contains(lower, "safe direct-git threshold"):
		out.Code = "LARGE_FILE_BLOCKED"
		out.Recoverable = true
		out.SuggestedAction = "Reduce the file size or configure Git LFS with .gitattributes."
	case strings.Contains(lower, "marked for git lfs"):
		out.Code = "GIT_LFS_REQUIRED"
		out.Recoverable = true
		out.SuggestedAction = "Install git-lfs and retry."
	case strings.Contains(lower, "requires pull requests") || strings.Contains(lower, "branch protection") && strings.Contains(lower, "will not bypass"):
		out.Code = "BRANCH_REQUIRES_PR"
		out.Recoverable = false
		out.SuggestedAction = "Use the repository's pull-request workflow; GitMake does not bypass protected branches."
	case strings.Contains(lower, "release tag") && strings.Contains(lower, "already exists"):
		out.Code = "TAG_CONFLICT"
		out.Recoverable = true
		out.SuggestedAction = "Choose a new release tag or review the existing tag manually."
	case strings.Contains(lower, "non-fast-forward") || strings.Contains(lower, "fetch first") || strings.Contains(lower, "rejected") && strings.Contains(lower, "push"):
		out.Code = "REMOTE_MOVED"
		out.Recoverable = true
		out.SuggestedAction = "Create a fresh GitMake plan; the remote branch changed."
	case strings.Contains(lower, "project identity mismatch"):
		out.Code = "PROJECT_IDENTITY_MISMATCH"
		out.Recoverable = false
		out.SuggestedAction = "Stop and verify the working directory, gitmake.json, source ZIP, and target repository. Do not override this binding automatically."
	case strings.Contains(lower, "destructive change blocked") || strings.Contains(lower, "destructive plan blocked") || strings.Contains(lower, "classified as destructive"):
		out.Code = "DESTRUCTIVE_CHANGE_BLOCKED"
		out.Recoverable = true
		out.SuggestedAction = "Review the plan provenance and deletion ratio. If intentional, a human must use --destructive explicitly."
	case strings.Contains(lower, "refusing to retarget repository"):
		out.Code = "PROJECT_SOURCE_MISMATCH"
		out.Recoverable = true
		out.SuggestedAction = "Verify the project directory and configured source. GitMake will not silently retarget an existing ZIP repository config to a different archive."
	case strings.Contains(lower, "approval token") || strings.Contains(lower, "one-shot approval"):
		out.Code = "APPROVAL_REQUIRED"
		out.Recoverable = true
		out.SuggestedAction = "Run `gitmake approve <plan_id>` and provide the one-shot token to the MCP apply tool."
	case strings.Contains(lower, "multiple source candidates") || strings.Contains(lower, "multiple zip"):
		out.Code = "SOURCE_AMBIGUOUS"
		out.Recoverable = true
		out.SuggestedAction = "Run `gitmake discover --json` or pass the source ZIP explicitly."
	case strings.Contains(lower, "no project zip") || strings.Contains(lower, "source zip") && strings.Contains(lower, "not found"):
		out.Code = "SOURCE_NOT_FOUND"
		out.Recoverable = true
		out.SuggestedAction = "Run GitMake inside a project folder, pass `.` explicitly, or provide a project ZIP."
	case strings.Contains(lower, "github authentication") || strings.Contains(lower, "gh auth login"):
		out.Code = "GH_AUTH_REQUIRED"
		out.Recoverable = true
		out.SuggestedAction = "Run `gh auth login`."
	case strings.Contains(lower, "github cli") && strings.Contains(lower, "not found"):
		out.Code = "GH_CLI_NOT_FOUND"
		out.Recoverable = true
		out.SuggestedAction = "Install GitHub CLI (`gh`)."
	case strings.Contains(lower, "git not found"):
		out.Code = "GIT_NOT_FOUND"
		out.Recoverable = true
		out.SuggestedAction = "Install Git."
	case strings.Contains(lower, "release") && strings.Contains(lower, "already exists"):
		out.Code = "RELEASE_EXISTS"
		out.Recoverable = true
		out.SuggestedAction = "Use release.on_existing=\"skip\" or \"resume\" if appropriate."
	case strings.Contains(lower, "plan") && strings.Contains(lower, "not found"):
		out.Code = "PLAN_NOT_FOUND"
		out.Recoverable = true
		out.SuggestedAction = "Create a new plan with `gitmake plan`."
	case strings.Contains(lower, "plan is stale") || strings.Contains(lower, "plan mismatch"):
		out.Code = "PLAN_STALE"
		out.Recoverable = true
		out.SuggestedAction = "Create a fresh plan and review it before applying."
	case strings.Contains(lower, "sha-256 mismatch") || strings.Contains(lower, "upgrade checksum") || strings.Contains(lower, "checksum for"):
		out.Code = "UPGRADE_INTEGRITY_FAILED"
		out.Recoverable = true
		out.SuggestedAction = "Do not install the downloaded build; retry later or verify the GitHub Release assets manually."
	case strings.Contains(lower, "config") || strings.Contains(lower, "schema_version"):
		out.Code = "CONFIG_INVALID"
		out.Recoverable = true
		out.SuggestedAction = "Fix gitmake.json or run `gitmake init` to recreate it."
	}
	return out
}

func fingerprintState(s *PipelineState) (string, error) {
	if s == nil {
		return "", fmt.Errorf("pipeline state is missing")
	}
	payload := struct {
		Repository       string         `json:"repository"`
		Visibility       string         `json:"visibility"`
		RemoteVisibility string         `json:"remote_visibility"`
		Mode             string         `json:"mode"`
		Branch           string         `json:"branch"`
		SourceMode       string         `json:"source_mode"`
		SourceSHA256     string         `json:"source_sha256"`
		BaseCommit       string         `json:"base_commit"`
		Changes          *ChangeCounts  `json:"changes"`
		Release          *ReleaseState  `json:"release"`
		Identity         *IdentityState `json:"identity"`
		Risk             *RiskState     `json:"risk"`
		ConfigSHA256     string         `json:"config_sha256"`
	}{
		Repository: s.Repository, Visibility: s.Visibility, RemoteVisibility: s.RemoteVisibility, Mode: s.Mode, Branch: s.Branch,
		SourceMode: s.SourceMode, SourceSHA256: s.SourceSHA256, BaseCommit: s.BaseCommit, Changes: s.Changes, Release: s.Release, Identity: s.Identity, Risk: s.Risk,
	}
	if s.Config != nil {
		payload.ConfigSHA256 = s.Config.SHA256
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func planFromState(id, cwd string, s *PipelineState) (planstore.Plan, error) {
	if s == nil || s.Changes == nil {
		return planstore.Plan{}, fmt.Errorf("plan could not be built from an incomplete pipeline")
	}
	fp, err := fingerprintState(s)
	if err != nil {
		return planstore.Plan{}, err
	}
	p := planstore.Plan{
		Schema: planstore.Schema, ID: id, WorkingDirectory: cwd,
		SourceMode: s.SourceMode, SourcePath: s.SourcePath, SourceSHA256: s.SourceSHA256,
		Repository: s.Repository, Visibility: s.Visibility, RemoteVisibility: s.RemoteVisibility, Mode: s.Mode, Branch: s.Branch,
		BaseCommit:  s.BaseCommit,
		Changes:     planstore.ChangeCounts{Added: s.Changes.Added, Modified: s.Changes.Modified, Deleted: s.Changes.Deleted},
		Fingerprint: fp,
	}
	if s.Config != nil {
		p.ConfigPath = s.Config.Path
		p.ConfigPersisted = s.Config.Persisted
		p.ConfigSHA256 = s.Config.SHA256
	}
	if s.Identity != nil {
		p.Identity = planstore.ProjectIdentity{Status: s.Identity.Status, ProjectID: s.Identity.ProjectID, Repository: s.Identity.Repository}
	}
	if s.Risk != nil {
		p.Risk = planstore.Risk{Level: s.Risk.Level, Destructive: s.Risk.Destructive, DeletionRatio: s.Risk.DeletionRatio, Deleted: s.Risk.Deleted, ManagedBaseline: s.Risk.ManagedBaseline, Reasons: append([]string(nil), s.Risk.Reasons...)}
	}
	p.ReviewNotes = []string{
		"Verify working_directory, config_path, source_mode, source_path, and repository before approval.",
		"A destructive plan requires a separate --destructive human approval and cannot use a normal approval token.",
	}
	if s.Release != nil {
		p.Release.Enabled = s.Release.Enabled
		p.Release.Tag = s.Release.Tag
		p.Release.NotesSHA256 = s.Release.NotesSHA256
		for _, d := range s.Release.AssetDigests {
			p.Release.Assets = append(p.Release.Assets, planstore.FileDigest{Name: d.Name, SHA256: d.SHA256})
		}
	}
	return p, nil
}

func releaseStateFromPlan(p releasePlan) (*ReleaseState, error) {
	s := &ReleaseState{
		Enabled: p.enabled,
		Tag:     p.spec.Tag,
		Assets:  len(p.spec.Assets),
		Skipped: p.skipExisting || !p.enabled,
		Resumed: p.resumeExisting,
		URL:     p.existingURL,
	}
	for _, path := range p.spec.Assets {
		sum, err := sha256File(path)
		if err != nil {
			return nil, fmt.Errorf("hash release asset %s: %w", path, err)
		}
		s.AssetDigests = append(s.AssetDigests, FileDigest{Name: filepathBase(path), SHA256: sum})
	}
	if p.spec.NotesFile != "" {
		sum, err := sha256File(p.spec.NotesFile)
		if err != nil {
			return nil, fmt.Errorf("hash release notes: %w", err)
		}
		s.NotesSHA256 = sum
	} else if p.spec.Notes != "" {
		sum := sha256.Sum256([]byte(p.spec.Notes))
		s.NotesSHA256 = hex.EncodeToString(sum[:])
	}
	return s, nil
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
