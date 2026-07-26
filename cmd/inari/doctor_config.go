// doctor_config.go owns doctor's config-drift reporting: settings that look
// configured but are not. it does NOT own detecting unknown keys (that is
// config.UnknownKeys, which knows the struct) or the rest of the doctor run.

package main

import (
	"strings"

	"github.com/mirageglobe/ai-inari/internal/config"
)

// reportConfigDrift warns about settings that look configured but do nothing.
// config.Load does no backfill for an existing file, so a config written before a
// field existed keeps the zero value, and keys that no longer exist are dropped by
// the decoder without a word: a config still holding the pre-consolidation
// models.worker leaves models.runner empty, and doctor then verifies only the
// thinker while still reporting green. advisory by design (warn, never fail) so
// the exit code keeps meaning "inari can run", and deliberately not a silent
// backfill, which would change which models doctor loads without being asked.
func reportConfigDrift(cfgPath string, cfg *config.Config) {
	if unknown, _ := config.UnknownKeys(cfgPath); len(unknown) > 0 {
		line("warn", "config keys", strings.Join(unknown, ", ")+"  (ignored: no such setting)")
	}
	if strings.TrimSpace(cfg.Models.Runner) == "" {
		line("warn", "runner", "not set  (the runner tier is unused; --models checks only the thinker)")
	}
}
