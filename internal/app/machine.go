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
	Stage           string        `json:"stage,omitempty"`
	CompletedStages []string      `json:"completed_stages,omitempty"`
	Mode            string        `json:"mode,omitempty"`
	Repository      string        `json:"repository,omitempty"`
	Visibility      string        `json:"visibility,omitempty"`
	Branch          string        `json:"branch,omitempty"`
	Source          string        `json:"source,omitempty"`
	Files           int           `json:"files,omitempty"`
	Changes         *ChangeCounts `json:"changes,omitempty"`
	RepositoryURL   string        `json:"repository_url,omitempty"`
	Release         *ReleaseState `json:"release,omitempty"`
	DryRun          bool          `json:"dry_run,omitempty"`
	ReadOnly        bool          `json:"read_only,omitempty"`
}

type ChangeCounts struct {
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

type ReleaseState struct {
	Enabled bool   `json:"enabled"`
	Tag     string `json:"tag,omitempty"`
	Assets  int    `json:"assets,omitempty"`
	Created bool   `json:"created,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	URL     string `json:"url,omitempty"`
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
	Kind    string `json:"kind"`
	Message string `json:"message"`
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
