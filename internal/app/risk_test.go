package app

import "testing"

func TestCalculateRiskBlocksMassManagedDeletion(t *testing.T) {
	r := calculateRisk(&ChangeCounts{Added: 18, Deleted: 73}, &SyncState{PriorManaged: 73}, "private", "public")
	if !r.Destructive {
		t.Fatal("expected destructive risk")
	}
	if r.DeletionRatio < 0.99 {
		t.Fatalf("ratio=%v", r.DeletionRatio)
	}
	if r.Level != "high" {
		t.Fatalf("level=%q", r.Level)
	}
}

func TestCalculateRiskSafeSmallChange(t *testing.T) {
	r := calculateRisk(&ChangeCounts{Added: 1, Modified: 2, Deleted: 1}, &SyncState{PriorManaged: 100}, "private", "private")
	if r.Destructive {
		t.Fatal("unexpected destructive risk")
	}
	if r.Level != "low" {
		t.Fatalf("level=%q", r.Level)
	}
}
