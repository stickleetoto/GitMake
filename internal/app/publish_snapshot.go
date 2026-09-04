package app

import (
	"fmt"
	"os"
	"path/filepath"

	"gitmake/internal/config"
	"gitmake/internal/securityscan"
)

// publishSnapshot is the reviewed copy of the source: a deterministic snapshot
// on disk, its file counts, and the security report that cleared it.
type publishSnapshot struct {
	// Workspace holds the snapshot and later the git clone.
	Workspace string
	// Path is the snapshot directory inside the workspace.
	Path string

	Files   int
	Ignored int

	Security securityscan.Report
}

// prepareSnapshot performs the PREPARE and SECURITY stages: materialise the
// source as a snapshot, prove it did not change while being copied, and run
// the security gate over it.
//
// The returned cleanup must be deferred by the caller. The workspace has to
// outlive this function -- everything downstream reads from it -- so the
// original `defer os.RemoveAll(work)` cannot simply move here. Keeping the
// deferral with the caller is the point of returning it.
func prepareSnapshot(o Options, source sourceSelection, cfg config.Config, sourceSHA256 string, hasLFS bool) (publishSnapshot, func(), error) {
	var snap publishSnapshot
	noCleanup := func() {}

	if o.State != nil {
		o.State.enter("PREPARE")
	}

	work, err := os.MkdirTemp("", "gitmake-*")
	if err != nil {
		return snap, noCleanup, err
	}
	cleanup := func() { os.RemoveAll(work) }
	if o.KeepTemp {
		fmt.Println("· Temporary workspace  " + work)
		cleanup = noCleanup
	}
	snap.Workspace = work
	snap.Path = filepath.Join(work, "snapshot")

	files, ignored, snapshotHash, err := snapshotSelectedSource(source, cfg, snap.Path)
	if err != nil {
		return snap, cleanup, err
	}
	if files == 0 {
		return snap, cleanup, fmt.Errorf("source %s contains no publishable regular files", source.Mode)
	}
	// The plan is bound to a source hash. A snapshot that hashes differently
	// means the source moved underneath the review, and the plan no longer
	// describes what would be published.
	if snapshotHash != sourceSHA256 {
		return snap, cleanup, fmt.Errorf("source changed while preparing the snapshot; create a fresh plan")
	}
	snap.Files = files
	snap.Ignored = ignored

	if o.State != nil {
		o.State.Files = files
		o.State.IgnoredFiles = ignored
		o.State.enter("SECURITY")
	}

	// The report is recorded even when the gate blocks: a caller that is told
	// only "blocked" cannot show the user which findings to act on.
	report, err := enforceSecurity(snap.Path, cfg, hasLFS)
	snap.Security = report
	if o.State != nil {
		o.State.Security = securityStateFromReport(report)
	}
	if err != nil {
		return snap, cleanup, err
	}
	return snap, cleanup, nil
}
