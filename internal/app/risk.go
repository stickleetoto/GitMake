package app

import (
	"fmt"
	"strings"
)

func calculateRisk(changes *ChangeCounts, sync *SyncState, configuredVisibility, remoteVisibility string) *RiskState {
	r := &RiskState{Level: "low"}
	if changes == nil {
		return r
	}
	r.Deleted = changes.Deleted
	if sync != nil {
		r.ManagedBaseline = sync.PriorManaged
	}
	if r.ManagedBaseline > 0 && changes.Deleted > 0 {
		r.DeletionRatio = float64(changes.Deleted) / float64(r.ManagedBaseline)
		if r.DeletionRatio > 1 {
			r.DeletionRatio = 1
		}
	}
	// Deletion-heavy updates are destructive when at least 10 managed files and
	// at least 30% of the managed baseline would disappear. This catches
	// project/context mistakes without forcing a destructive ceremony for tiny repos.
	if r.ManagedBaseline > 0 && changes.Deleted >= 10 && r.DeletionRatio >= 0.30 {
		r.Level = "high"
		r.Destructive = true
		r.Reasons = append(r.Reasons, fmt.Sprintf("%d of %d previously managed files would be deleted (%.1f%%)", changes.Deleted, r.ManagedBaseline, r.DeletionRatio*100))
	}
	if strings.TrimSpace(remoteVisibility) != "" && !strings.EqualFold(configuredVisibility, remoteVisibility) {
		if r.Level == "low" {
			r.Level = "medium"
		}
		r.Reasons = append(r.Reasons, fmt.Sprintf("config visibility %s differs from remote visibility %s; updates do not change remote visibility", configuredVisibility, strings.ToLower(remoteVisibility)))
	}
	return r
}
