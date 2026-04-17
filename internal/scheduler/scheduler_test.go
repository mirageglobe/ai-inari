package scheduler

import (
	"testing"
)

func TestAcquireRelease(t *testing.T) {
	s := New(2048)

	if err := s.Acquire("worker"); err != nil {
		t.Fatalf("Acquire worker: %v", err)
	}
	if s.Used() != 500 {
		t.Errorf("used = %d, want 500", s.Used())
	}
	if s.Free() != 1548 {
		t.Errorf("free = %d, want 1548", s.Free())
	}

	s.Release("worker")
	if s.Used() != 0 {
		t.Errorf("used after release = %d, want 0", s.Used())
	}
}

func TestBudgetExceeded(t *testing.T) {
	s := New(100)
	if err := s.Acquire("worker"); err == nil {
		t.Error("expected error when budget exceeded, got nil")
	}
}

func TestUnknownTier(t *testing.T) {
	s := New(8192)
	if err := s.Acquire("ghost"); err == nil {
		t.Error("expected error for unknown tier, got nil")
	}
}
