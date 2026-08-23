package app

import (
	"time"

	"gitmake/internal/approval"
	"gitmake/internal/opjournal"
	"gitmake/internal/oplock"
	"gitmake/internal/planstore"
)

func runHousekeeping() {
	_ = approval.Cleanup()
	_ = oplock.Cleanup()
	_ = planstore.Cleanup(30 * 24 * time.Hour)
	_ = opjournal.Cleanup(30 * 24 * time.Hour)
}
