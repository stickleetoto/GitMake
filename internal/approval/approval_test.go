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
