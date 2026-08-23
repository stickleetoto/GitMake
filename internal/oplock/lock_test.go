package oplock

import "testing"

func TestLockPreventsConcurrentAcquire(t *testing.T) {
	l, err := Acquire("plan:test-lock")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if _, err := Acquire("plan:test-lock"); err == nil {
		t.Fatal("expected second acquire to fail")
	}
	l.Release()
	l2, err := Acquire("plan:test-lock")
	if err != nil {
		t.Fatal(err)
	}
	l2.Release()
}
