package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/github"
	"gitmake/internal/gitops"
)

// A publish used to end by reporting what it had asked GitHub to do. Nothing
// asked GitHub what it actually did.
//
// That is the shape of failure this project keeps finding: a command returns
// zero, the output says the work happened, and nobody looked. The staged
// upgrade helper reported success for three releases while doing nothing at
// all. So the last thing a publish does now is ask the remote where it is and
// compare that with what was pushed.
//
// The distinction that matters here is between a check that ran and disagreed,
// and a check that could not run. The first is a failure: GitHub does not hold
// what the user approved. The second is not, because a network hiccup while
// reading back a push that already succeeded is not a reason to report the
// publish as broken -- it is a reason to say it was not confirmed.

// verifyPublish is the VERIFY stage. It reports an error only when a check ran
// and disagreed.
func verifyPublish(o Options, repoDir, branch, target string, release releasePlan, git gitops.Client, gh github.Client) error {
	if o.State == nil || o.DryRun {
		return nil
	}
	o.State.enter("VERIFY")
	v := &VerificationState{}
	o.State.Verification = v

	// The commit. An empty one means the run did not push anything -- an
	// already-up-to-date update -- and there is nothing to confirm.
	if o.State.Commit != "" {
		remote, err := git.RemoteHeadSHA(repoDir, branch)
		switch {
		case err != nil:
			fmt.Printf("· Not verified          could not read %s from the remote: %v\n", branch, err)
		case remote == "":
			v.Checked = true
			v.Problems = append(v.Problems, fmt.Sprintf("branch %s does not exist on the remote after a successful push", branch))
		default:
			v.Checked = true
			v.RemoteCommit = remote
			v.CommitMatches = remote == o.State.Commit
			if !v.CommitMatches {
				v.Problems = append(v.Problems, fmt.Sprintf(
					"remote %s is at %.12s, not the commit this publish pushed (%.12s)", branch, remote, o.State.Commit))
			}
		}
	}

	// The release. Only a release this run actually created or resumed is
	// checked: an existing one that was left alone is not this run's claim.
	if st := o.State.Release; st != nil && (st.Created || st.Resumed) && st.Tag != "" {
		info, found, err := gh.Release(target, st.Tag)
		switch {
		case err != nil:
			fmt.Printf("· Not verified          could not read release %s: %v\n", st.Tag, err)
		case !found:
			v.Checked = true
			v.Problems = append(v.Problems, fmt.Sprintf("release %s was reported created but GitHub does not have it", st.Tag))
		default:
			v.Checked = true
			v.ReleaseAssets = len(info.Assets)
			v.AssetsMatch = true
			for _, problem := range compareAssets(release.spec.Assets, info.Assets) {
				v.AssetsMatch = false
				v.Problems = append(v.Problems, problem)
			}
		}
	}

	reportVerification(v)
	if len(v.Problems) > 0 {
		return fmt.Errorf("publish could not be verified: %s", strings.Join(v.Problems, "; "))
	}
	return nil
}

// compareAssets checks that every uploaded file arrived, at the size it was
// sent. A size mismatch is how a truncated upload shows up; without this the
// release would be reported complete.
func compareAssets(local []string, remote []github.ReleaseAsset) []string {
	if len(local) == 0 {
		return nil
	}
	sizes := make(map[string]int64, len(remote))
	for _, a := range remote {
		sizes[a.Name] = a.Size
	}
	var problems []string
	for _, path := range local {
		name := filepath.Base(path)
		size, ok := sizes[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("asset %s is missing from the release", name))
			continue
		}
		st, err := os.Stat(path)
		if err != nil {
			// The file is gone from disk; its size cannot be compared. Not a
			// disagreement, so it is not reported as one.
			continue
		}
		// A zero size from the API means the field was not reported rather
		// than that the asset is empty, so it is not treated as a mismatch.
		if size != 0 && size != st.Size() {
			problems = append(problems, fmt.Sprintf(
				"asset %s is %d bytes on the release but %d bytes locally", name, size, st.Size()))
		}
	}
	return problems
}

func reportVerification(v *VerificationState) {
	if !v.Checked {
		return
	}
	if len(v.Problems) > 0 {
		fmt.Println("! Verification failed")
		for _, p := range v.Problems {
			fmt.Println("  " + p)
		}
		return
	}
	switch {
	case v.RemoteCommit != "" && v.ReleaseAssets > 0:
		fmt.Printf("✓ Verified              remote at %.12s · %d assets\n", v.RemoteCommit, v.ReleaseAssets)
	case v.RemoteCommit != "":
		fmt.Printf("✓ Verified              remote at %.12s\n", v.RemoteCommit)
	case v.ReleaseAssets > 0:
		fmt.Printf("✓ Verified              %d release assets\n", v.ReleaseAssets)
	}
}
