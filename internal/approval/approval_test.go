package approval

import "testing"

func TestOneShotApproval(t *testing.T) {
	token, _, err := Create("gm_testapproval")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate("gm_testapproval", token); err != nil {
		t.Fatal(err)
	}
	if err := Consume("gm_testapproval", token); err != nil {
		t.Fatal(err)
	}
	if err := Validate("gm_testapproval", token); err == nil {
		t.Fatal("used token should be rejected")
	}
	if err := Validate("gm_testapproval", token+"00"); err == nil {
		t.Fatal("wrong token should be rejected")
	}
}

func TestDestructiveApprovalRecord(t *testing.T) {
	planID := "gm_destructive_test"
	token, _, err := Create(planID, true)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ValidateRecord(planID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Destructive {
		t.Fatal("expected destructive approval class")
	}
}

func TestTokenlessGrantBindingAndConsume(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	planID := "gm_tokenless_test"
	binding := Binding{Fingerprint: "fp", SourceSHA256: "src", ConfigSHA256: "cfg", Repository: "owner/repo"}
	r, err := CreateGrant(planID, binding, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.TokenHash != "" {
		t.Fatal("tokenless grant must not contain token hash")
	}
	if _, err := ValidateGrant(planID, binding); err != nil {
		t.Fatal(err)
	}
	bad := binding
	bad.Repository = "owner/other"
	if _, err := ValidateGrant(planID, bad); err == nil {
		t.Fatal("binding mismatch should be rejected")
	}
	if err := ConsumeGrant(planID, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateGrant(planID, binding); err == nil {
		t.Fatal("consumed grant should be rejected")
	}
}

func TestCleanupKeepsFreshUnusedGrant(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	planID := "gm_cleanup_fresh"
	binding := Binding{Fingerprint: "fp", SourceSHA256: "src", Repository: "owner/repo"}
	if _, err := CreateGrant(planID, binding, false); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateGrant(planID, binding); err != nil {
		t.Fatalf("fresh grant was removed: %v", err)
	}
}

func TestConsumedGrantCannotBeRemintedForSamePlan(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	planID := "gm_no_replay"
	binding := Binding{Fingerprint: "fp", SourceSHA256: "src", ConfigSHA256: "cfg", Repository: "owner/repo"}
	if _, err := CreateGrant(planID, binding, false); err != nil {
		t.Fatal(err)
	}
	if err := ConsumeGrant(planID, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateGrant(planID, binding, false); err == nil {
		t.Fatal("consumed approval must not be re-minted for the same reviewed plan")
	}
}
