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
