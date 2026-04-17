package scheduler

import "fmt"

// Tier memory costs in MB.
var tierCost = map[string]int{
	"sensor":  100,
	"worker":  500,
	"thinker": 1024,
}

// Scheduler is a semaphore-based concurrency throttle budgeted by memory (MB).
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

// Acquire reserves memory for a tier. Blocks until budget is available.
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

// Release frees memory reserved for a tier.
func (s *Scheduler) Release(tier string) {
	cost := tierCost[tier]

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	s.used -= cost
	if s.used < 0 {
		s.used = 0
	}
}

func (s *Scheduler) Used() int  { return s.used }
func (s *Scheduler) Free() int  { return s.budget - s.used }
