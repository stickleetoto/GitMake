package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type PipelineState struct {
	Stage            string          `json:"stage,omitempty"`
	CompletedStages  []string        `json:"completed_stages,omitempty"`
	Mode             string          `json:"mode,omitempty"`
	Repository       string          `json:"repository,omitempty"`
	Visibility       string          `json:"visibility,omitempty"`
	RemoteVisibility string          `json:"remote_visibility,omitempty"`
	Branch           string          `json:"branch,omitempty"`
	Source           string          `json:"source,omitempty"`
	SourceMode       string          `json:"source_mode,omitempty"`
	SourcePath       string          `json:"source_path,omitempty"`
	SourceSHA256     string          `json:"source_sha256,omitempty"`
	IgnoredFiles     int             `json:"ignored_files,omitempty"`
	BaseCommit       string          `json:"base_commit,omitempty"`
	PlanID           string          `json:"plan_id,omitempty"`
	Files            int             `json:"files,omitempty"`
	Changes          *ChangeCounts   `json:"changes,omitempty"`
	RepositoryURL    string          `json:"repository_url,omitempty"`
	Release          *ReleaseState   `json:"release,omitempty"`
	DryRun           bool            `json:"dry_run,omitempty"`
	ReadOnly         bool            `json:"read_only,omitempty"`
	Config           *ConfigState    `json:"config,omitempty"`
	Discovery        *DiscoveryState `json:"discovery,omitempty"`
	Security         *SecurityState  `json:"security,omitempty"`
	Sync             *SyncState      `json:"sync,omitempty"`
	Identity         *IdentityState  `json:"identity,omitempty"`
	Risk             *RiskState      `json:"risk,omitempty"`
}

type SecurityState struct {
	SecretScan   bool              `json:"secret_scan"`
	ScannedFiles int               `json:"scanned_files"`
	Findings     []SecurityFinding `json:"findings,omitempty"`
	LargeFiles   []LargeFileState  `json:"large_files,omitempty"`
	LFSRequired  bool              `json:"lfs_required"`
	Blocking     bool              `json:"blocking"`
}

type SecurityFinding struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type LargeFileState struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	LFSMarked bool   `json:"lfs_marked"`
	Blocking  bool   `json:"blocking"`
}

type SyncState struct {
	Mode         string   `json:"mode"`
	PriorManaged int      `json:"prior_managed"`
	ManagedFiles int      `json:"managed_files"`
	FirstAdopt   bool     `json:"first_adopt"`
	Deleted      []string `json:"deleted,omitempty"`
	Preserved    []string `json:"preserved,omitempty"`
}

type IdentityState struct {
	Schema     string `json:"schema,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Repository string `json:"repository,omitempty"`
	Status     string `json:"status"`
}

type RiskState struct {
	Level           string   `json:"level"`
	Destructive     bool     `json:"destructive"`
	DeletionRatio   float64  `json:"deletion_ratio"`
	Deleted         int      `json:"deleted"`
	ManagedBaseline int      `json:"managed_baseline"`
	Reasons         []string `json:"reasons,omitempty"`
}

type ConfigState struct {
	Source    string `json:"source"`
	Persisted bool   `json:"persisted"`
	Path      string `json:"path,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type DiscoveryState struct {
	SelectedSource        string               `json:"selected_source,omitempty"`
	SourceConfidence      string               `json:"source_confidence,omitempty"`
	SourceConfidenceScore float64              `json:"source_confidence_score,omitempty"`
	SelectedSourceScore   int                  `json:"selected_source_score,omitempty"`
	SelectedEvidence      []string             `json:"selected_evidence,omitempty"`
	CandidateDetails      []DiscoveryCandidate `json:"candidate_details,omitempty"`
	ReleaseAssets         []string             `json:"release_assets,omitempty"`
	Unknown               []string             `json:"unknown,omitempty"`
	NeedsInput            bool                 `json:"needs_input"`
	Reason                string               `json:"reason,omitempty"`
	Candidates            []string             `json:"candidates,omitempty"`
}

type DiscoveryCandidate struct {
	Name           string   `json:"name"`
	Classification string   `json:"classification"`
	SourceScore    int      `json:"source_score"`
	AssetScore     int      `json:"asset_score"`
	Reasons        []string `json:"reasons,omitempty"`
}

type FileDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type ChangeCounts struct {
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

type ReleaseState struct {
	Enabled      bool         `json:"enabled"`
	Tag          string       `json:"tag,omitempty"`
	Assets       int          `json:"assets,omitempty"`
	AssetDigests []FileDigest `json:"asset_digests,omitempty"`
	NotesSHA256  string       `json:"notes_sha256,omitempty"`
	Created      bool         `json:"created,omitempty"`
	Resumed      bool         `json:"resumed,omitempty"`
	Skipped      bool         `json:"skipped,omitempty"`
	URL          string       `json:"url,omitempty"`
}

type MachineResult struct {
	Schema   string         `json:"schema"`
	OK       bool           `json:"ok"`
	Version  string         `json:"version"`
	Command  string         `json:"command"`
	ExitCode int            `json:"exit_code"`
	Pipeline *PipelineState `json:"pipeline,omitempty"`
	Output   string         `json:"output,omitempty"`
	Error    *MachineError  `json:"error,omitempty"`
}

type MachineError struct {
	Kind            string `json:"kind"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	Stage           string `json:"stage,omitempty"`
	Recoverable     bool   `json:"recoverable"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

func newPipeline(o Options) *PipelineState {
	return &PipelineState{DryRun: o.DryRun, ReadOnly: o.ReadOnly}
}

func (p *PipelineState) enter(stage string) {
	if p == nil {
		return
	}
	if p.Stage != "" && (len(p.CompletedStages) == 0 || p.CompletedStages[len(p.CompletedStages)-1] != p.Stage) {
		p.CompletedStages = append(p.CompletedStages, p.Stage)
	}
	p.Stage = stage
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON output: %w", err)
	}
	return nil
}

func captureOutput(fn func() error) (string, error) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		return "", err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		_ = rOut.Close()
		_ = wOut.Close()
		return "", err
	}
	os.Stdout, os.Stderr = wOut, wErr

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(&outBuf, rOut) }()
	go func() { defer wg.Done(); _, _ = io.Copy(&errBuf, rErr) }()

	runErr := fn()
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	wg.Wait()
	_ = rOut.Close()
	_ = rErr.Close()

	combined := outBuf.String()
	if errBuf.Len() > 0 {
		if combined != "" && combined[len(combined)-1] != '\n' {
			combined += "\n"
		}
		combined += errBuf.String()
	}
	return combined, runErr
}
