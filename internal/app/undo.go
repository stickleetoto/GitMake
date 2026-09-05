package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/github"
	"gitmake/internal/gitops"
	"gitmake/internal/gmerr"
	"gitmake/internal/history"
	"gitmake/internal/oplock"
	"gitmake/internal/planstore"
	"gitmake/internal/runner"
	"gitmake/internal/syncer"
)

// GitMake could publish but never return. `gitmake history` said what had
// happened and left the user to work out the rest by hand.
//
// Undo is deliberately the smallest thing that actually helps: it adds a
// commit that restores the previous content. It does not reset, force-push, or
// delete anything, because the safety contract forbids all three and because
// none of them would achieve more. Removing a commit from a branch does not
// remove it from GitHub: the objects stay reachable by SHA, and forks, clones,
// caches and CI logs are past reach entirely.
//
// That last point is the one this command must never let a user misread. See
// warnPublishedContentRemains.

// undoTarget is the publish an undo would revert, with the reason it cannot be
// when there is one.
type undoTarget struct {
	entry history.Entry
	why   string
}

// findUndoTarget picks the most recent publish that can be returned.
//
// A dry run changed nothing, an already-undone entry would be reverted twice,
// and a run that created the repository has no earlier state to go back to.
// Each of those is reported as itself rather than as "nothing found", because
// the user is entitled to know which one they hit.
func findUndoTarget(entries []history.Entry) undoTarget {
	for _, e := range entries {
		if !e.OK || e.DryRun || e.ReadOnly {
			continue
		}
		if e.Command != "publish" && e.Command != "apply" {
			continue
		}
		if e.Undone {
			return undoTarget{why: fmt.Sprintf("the last publish to %s has already been undone", e.Repository)}
		}
		if e.RepoCreated {
			return undoTarget{why: fmt.Sprintf(
				"the last publish created %s, so there is no earlier state to return to; GitMake does not delete repositories, so removing it is yours to do on GitHub", e.Repository)}
		}
		if e.Commit == "" {
			return undoTarget{why: fmt.Sprintf(
				"the last publish to %s changed nothing, so there is nothing to undo", e.Repository)}
		}
		return undoTarget{entry: e}
	}
	return undoTarget{why: "no completed publish was found in this machine's GitMake history"}
}

func runUndo(o Options) error {
	entries, err := history.List(50)
	if err != nil {
		return fmt.Errorf("read GitMake history: %w", err)
	}
	target := findUndoTarget(entries)
	if target.why != "" {
		return gmerr.New(gmerr.NothingToUndo, "nothing to undo: %s", target.why)
	}
	e := target.entry

	if o.State != nil {
		o.State.Repository = e.Repository
		o.State.Branch = e.Branch
		o.State.Mode = "UNDO"
		o.State.DryRun = o.DryRun
	}

	fmt.Printf("GitMake Undo · %s\n\n", Version)
	fmt.Printf("· Repository            %s\n", e.Repository)
	fmt.Printf("· Branch                %s\n", firstNonEmpty(e.Branch, "(default)"))
	fmt.Printf("· Publish to undo       %.12s · +%d ~%d -%d\n", e.Commit, e.Added, e.Modified, e.Deleted)
	fmt.Printf("· Published at          %s\n", e.Time.Local().Format("2006-01-02 15:04:05"))
	if e.ReleaseTag != "" {
		fmt.Printf("· Release               %s · left in place\n", e.ReleaseTag)
	}

	run := runner.Runner{Verbose: o.Verbose}
	git := gitops.Client{Run: run}
	gh := github.Client{Run: run}
	if err := gh.Preflight(); err != nil {
		return err
	}

	// The same lock a publish takes: an undo is a publish of the previous
	// state, and two of them running at once on one repository is the same
	// hazard.
	repoLock, lockErr := oplock.Acquire("repo:" + strings.ToLower(e.Repository))
	if lockErr != nil {
		return lockErr
	}
	defer repoLock.Release()

	work, err := os.MkdirTemp("", "gitmake-undo-*")
	if err != nil {
		return err
	}
	if o.KeepTemp {
		fmt.Println("· Temporary workspace   " + work)
	} else {
		defer os.RemoveAll(work)
	}
	repoDir := filepath.Join(work, "repo")
	if err := gh.Clone(e.Repository, repoDir); err != nil {
		return err
	}

	branch := e.Branch
	if branch == "" {
		info, ok, infoErr := gh.Repo(splitTarget(e.Repository))
		if infoErr != nil {
			return infoErr
		}
		if !ok {
			return gmerr.New(gmerr.NothingToUndo, "repository %s no longer exists", e.Repository)
		}
		branch = info.DefaultBranch()
	}

	exists, err := git.CommitExists(repoDir, e.Commit)
	if err != nil {
		return err
	}
	if !exists {
		return gmerr.New(gmerr.RemoteMoved,
			"commit %.12s is not in %s any more; GitMake will not guess what to revert", e.Commit, e.Repository)
	}

	// The published commit must still be the tip. Reverting under somebody
	// else's later work would undo a state that no longer exists, and deciding
	// what they meant is not GitMake's call to make.
	remote, err := git.RemoteHeadSHA(repoDir, branch)
	if err != nil {
		return err
	}
	if remote == "" {
		return gmerr.New(gmerr.RemoteMoved, "branch %s no longer exists in %s", branch, e.Repository)
	}
	if remote != e.Commit {
		return gmerr.New(gmerr.RemoteMoved,
			"%s has moved on since that publish: the branch is at %.12s, not %.12s. "+
				"Undoing now would revert on top of work GitMake did not publish; review it and revert by hand if that is what you want",
			branch, remote, e.Commit)
	}
	fmt.Printf("✓ Branch unchanged      %s still at %.12s\n", branch, remote)

	if o.DryRun {
		fmt.Println("· Dry run               no commit or push")
		warnPublishedContentRemains(e)
		return nil
	}

	if err := confirmUndo(o, e, repoDir); err != nil {
		return err
	}

	if err := git.EnsureIdentity(repoDir); err != nil {
		return err
	}
	message := fmt.Sprintf("Undo GitMake publish %.12s", e.Commit)
	if err := git.Revert(repoDir, e.Commit, message); err != nil {
		return err
	}
	if err := git.Push(repoDir, branch); err != nil {
		return err
	}
	undoCommit, err := git.HeadSHA(repoDir)
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.Commit = undoCommit
	}
	fmt.Printf("✓ Reverted              %.12s\n", e.Commit)
	fmt.Printf("✓ Pushed                %s · %.12s\n", branch, undoCommit)

	// The same confirmation a publish gets: ask the remote rather than trust
	// the push that just returned.
	if o.State != nil {
		o.State.enter("VERIFY")
		v := &VerificationState{}
		o.State.Verification = v
		switch after, verifyErr := git.RemoteHeadSHA(repoDir, branch); {
		case verifyErr != nil:
			fmt.Printf("· Not verified          could not read %s from the remote: %v\n", branch, verifyErr)
		default:
			v.Checked = true
			v.RemoteCommit = after
			v.CommitMatches = after == undoCommit
			if !v.CommitMatches {
				v.Problems = append(v.Problems, fmt.Sprintf(
					"remote %s is at %.12s, not the revert this run pushed (%.12s)", branch, after, undoCommit))
				reportVerification(v)
				return fmt.Errorf("undo could not be verified: %s", strings.Join(v.Problems, "; "))
			}
			reportVerification(v)
		}
	}

	if err := history.MarkUndone(e.ID); err != nil {
		// The revert landed; failing the command now would misreport it.
		fmt.Printf("· History not updated   %v\n", err)
	}
	warnPublishedContentRemains(e)
	return nil
}

// undoPrompter is the conversation confirmUndo has. It is a variable so a test
// can decide whether a human is available, rather than inheriting whatever
// stdin the test runner happened to be given.
var undoPrompter prompter = terminalPrompter{}

// confirmUndo applies the same ceremony rules a publish does.
//
// An undo is a publish of the previous state, so it gets the same friction. It
// is not a back door around the confirmation table.
func confirmUndo(o Options, e history.Entry, repoDir string) error {
	// Reverting a publish removes what that publish added, so the files it
	// created are the deletions here and vice versa.
	changes := &ChangeCounts{Added: e.Deleted, Modified: e.Modified, Deleted: e.Added}
	// Risk is a ratio, so it needs a denominator. Passing no sync state here
	// left ManagedBaseline at zero, which made the destructive rule
	// unreachable: an undo removing five hundred files would have been offered
	// as an ordinary [Y/n] question. The baseline comes from the manifest the
	// previous run wrote.
	var sync *SyncState
	if baseline, ok, err := syncer.ManagedBaseline(repoDir); err == nil && ok {
		sync = &SyncState{PriorManaged: baseline}
	}
	risk := calculateRisk(changes, sync, "", "")
	if o.State != nil {
		o.State.Changes = changes
		o.State.Risk = risk
	}
	plan := planstore.Plan{
		ID:   firstNonEmpty(e.ID, e.Commit),
		Mode: "UNDO",
		Risk: planstore.Risk{Level: risk.Level, Destructive: risk.Destructive},
	}
	confirmed, _, err := confirmPlan(plan, o.Yes, undoPrompter)
	if err != nil {
		return err
	}
	if !confirmed {
		return gmerr.New(gmerr.ApprovalRequired, "undo cancelled")
	}
	return nil
}

// warnPublishedContentRemains is the honesty this command lives or dies by.
//
// Reverting adds a commit. It does not unpublish: the previous contents stay
// reachable by SHA, in forks and clones, and in anything that already read
// them. A user who believes an undo removed a leaked credential is worse off
// than one who never ran it, so GitMake says so every time rather than only
// when it thinks it saw a secret.
func warnPublishedContentRemains(e history.Entry) {
	fmt.Println()
	fmt.Println("! What was published stays published.")
	fmt.Printf("  The reverted contents remain reachable in Git history and through the\n")
	fmt.Printf("  GitHub API, and in any fork, clone or CI log that already has them.\n")
	fmt.Printf("  If a credential was published, undoing does not unpublish it: rotate it.\n")
	if e.ReleaseTag != "" {
		fmt.Printf("  Release %s was left in place; GitMake does not delete releases.\n", e.ReleaseTag)
	}
}

// splitTarget separates "owner/name". A stored repository always has both.
func splitTarget(target string) (string, string) {
	owner, name, _ := strings.Cut(target, "/")
	return owner, name
}
