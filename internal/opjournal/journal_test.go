package opjournal

import "testing"

func TestInterruptedJournalDetection(t *testing.T) {
	plan := "gm_journal_test"
	interrupted, err := Begin(plan, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if interrupted {
		t.Fatal("first begin must not be interrupted")
	}
	interrupted, err = Begin(plan, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !interrupted {
		t.Fatal("second begin should detect interrupted prior run")
	}
	if err := Finish(plan, nil); err != nil {
		t.Fatal(err)
	}
}
