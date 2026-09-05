// Package gmerr carries GitMake's machine-readable error codes on the error
// value itself.
//
// The `--json` error codes are part of the frozen v1 contract, but they used to
// be recovered by matching substrings of the human-readable message. Two things
// followed from that. Rewording any error anywhere in the codebase could
// silently reclassify it, with no test to notice. And the broadest patterns
// swallowed unrelated failures: every error whose text merely contained the
// word "config" was reported as CONFIG_INVALID, including "read-only mode
// blocks gitmake config write" and an I/O failure from "hash config: ...".
//
// An error built here states its own code. Message text becomes free to edit.
package gmerr

import (
	"errors"
	"fmt"
)

// Code is a documented GitMake machine error code. The values are part of the
// v1 contract and must not be repurposed.
type Code string

const (
	SecretDetected          Code = "SECRET_DETECTED"
	LargeFileBlocked        Code = "LARGE_FILE_BLOCKED"
	GitLFSRequired          Code = "GIT_LFS_REQUIRED"
	BranchRequiresPR        Code = "BRANCH_REQUIRES_PR"
	TagConflict             Code = "TAG_CONFLICT"
	RemoteMoved             Code = "REMOTE_MOVED"
	ProjectIdentityMismatch Code = "PROJECT_IDENTITY_MISMATCH"
	DestructiveBlocked      Code = "DESTRUCTIVE_CHANGE_BLOCKED"
	ProjectSourceMismatch   Code = "PROJECT_SOURCE_MISMATCH"
	ApprovalRequired        Code = "APPROVAL_REQUIRED"
	SourceAmbiguous         Code = "SOURCE_AMBIGUOUS"
	SourceNotFound          Code = "SOURCE_NOT_FOUND"
	GHAuthRequired          Code = "GH_AUTH_REQUIRED"
	GHCLINotFound           Code = "GH_CLI_NOT_FOUND"
	GitNotFound             Code = "GIT_NOT_FOUND"
	ReleaseExists           Code = "RELEASE_EXISTS"
	PlanNotFound            Code = "PLAN_NOT_FOUND"
	PlanStale               Code = "PLAN_STALE"
	UpgradeIntegrityFailed  Code = "UPGRADE_INTEGRITY_FAILED"
	ConfigInvalid           Code = "CONFIG_INVALID"
	// NothingToUndo is additive in v1.3. It reports that `gitmake undo` found
	// no publish it can return, which is an answer rather than a malfunction.
	NothingToUndo Code = "NOTHING_TO_UNDO"
)

// guidance is the single source of truth for how each code is reported. It
// used to be inlined in the classifier, where it was reachable only through
// the message matching it was meant to describe.
type guidance struct {
	Recoverable bool
	Action      string
}

var guidanceByCode = map[Code]guidance{
	SecretDetected:          {true, "Remove the secret from the selected source or explicitly allow a safe fixture path in security.allow_secret_paths."},
	LargeFileBlocked:        {true, "Reduce the file size or configure Git LFS with .gitattributes."},
	GitLFSRequired:          {true, "Install git-lfs and retry."},
	BranchRequiresPR:        {false, "Use the repository's pull-request workflow; GitMake does not bypass protected branches."},
	TagConflict:             {true, "Choose a new release tag or review the existing tag manually."},
	RemoteMoved:             {true, "Create a fresh GitMake plan; the remote branch changed."},
	ProjectIdentityMismatch: {false, "Stop and verify the working directory, gitmake.json, source ZIP, and target repository. Do not override this binding automatically."},
	DestructiveBlocked:      {true, "Review the plan provenance and deletion ratio. If intentional, a human must use --destructive explicitly."},
	ProjectSourceMismatch:   {true, "Verify the project directory and configured source. GitMake will not silently retarget an existing ZIP repository config to a different archive."},
	ApprovalRequired:        {true, "Run `gitmake approve` in the reviewed project directory. GitMake stores a short-lived local single-use grant; no token copy is required."},
	SourceAmbiguous:         {true, "Run `gitmake discover --json` or pass the source ZIP explicitly."},
	SourceNotFound:          {true, "Run GitMake inside a project folder, pass `.` explicitly, or provide a project ZIP."},
	GHAuthRequired:          {true, "Run `gh auth login`."},
	GHCLINotFound:           {true, "Install GitHub CLI (`gh`)."},
	GitNotFound:             {true, "Install Git."},
	ReleaseExists:           {true, "Use release.on_existing=\"skip\" or \"resume\" if appropriate."},
	PlanNotFound:            {true, "Create a new plan with `gitmake plan`."},
	PlanStale:               {true, "Create a fresh plan and review it before applying."},
	UpgradeIntegrityFailed:  {true, "Do not install the downloaded build; retry later or verify the GitHub Release assets manually."},
	NothingToUndo:           {true, "Run `gitmake history` to see what GitMake has published from this machine. A publish that created a repository, changed nothing, or was already undone cannot be undone."},
	ConfigInvalid:           {true, "Fix gitmake.json or run `gitmake init` to recreate it."},
}

// Guidance reports how a code should be presented. ok is false for a code with
// no registered guidance, which the classifier treats as a plain runtime error.
func Guidance(code Code) (recoverable bool, action string, ok bool) {
	g, found := guidanceByCode[code]
	return g.Recoverable, g.Action, found
}

// Codes lists every documented code. Tests use it to prove that each one is
// still reachable and still reported with its documented guidance.
func Codes() []Code {
	out := make([]Code, 0, len(guidanceByCode))
	for code := range guidanceByCode {
		out = append(out, code)
	}
	return out
}

// Error is a GitMake failure that knows its own machine code.
type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil && e.Msg == "" {
		return e.Err.Error()
	}
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

// New builds a coded error with its own message.
func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap attaches a code to an existing error, preserving it for errors.Is and
// errors.As. A nil cause yields nil so call sites can wrap unconditionally.
func Wrap(code Code, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Err: err}
}

// CodeOf reports the code carried by err, or "" when it carries none.
func CodeOf(err error) Code {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ""
}
