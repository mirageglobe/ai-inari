// Package scheduler enforces the memory budget across concurrent Ollama sessions.
// It uses a semaphore to gate how many sessions run in parallel based on configured MB limits.
package scheduler

import "fmt"

// Tier memory costs in MB.
var tierCost = map[string]int{
	"sensor":  100,
	"worker":  500,
	"thinker": 1024,
}

// Scheduler is a memory-budget throttle for concurrent Ollama sessions.
// sem is a capacity-1 channel used as a mutex over `used`; it serialises Acquire/Release
// without importing sync, keeping the critical section visually explicit.
type Scheduler struct {
	budget int
	used   int
	sem    chan struct{}
}

func New(budgetMB int) *Scheduler {
	return &Scheduler{
		budget: budgetMB,
		sem:    make(chan struct{}, 1),
	}
}

// Acquire reserves memory for a tier. Returns an error immediately if the budget is exceeded —
// it does not block waiting for other sessions to release; the caller decides whether to retry.
func (s *Scheduler) Acquire(tier string) error {
	cost, ok := tierCost[tier]
	if !ok {
		return fmt.Errorf("scheduler: unknown tier %q", tier)
	}

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	if s.used+cost > s.budget {
		return fmt.Errorf("scheduler: insufficient memory budget (%d MB used, %d MB cost, %d MB limit)", s.used, cost, s.budget)
	}
	s.used += cost
	return nil
}

// Release frees memory reserved for a tier. The floor guard prevents `used` going negative
// if Release is called without a matching Acquire (e.g. during error recovery).
func (s *Scheduler) Release(tier string) {
	cost := tierCost[tier]

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	s.used -= cost
	if s.used < 0 {
		s.used = 0
	}
}

func (s *Scheduler) Used() int { return s.used }
func (s *Scheduler) Free() int { return s.budget - s.used }
