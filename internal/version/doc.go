// Package version holds the single source of truth for the application version.
//
// it owns:
//   - the Version constant; all callers import this package directly.
//
// it does NOT own:
//   - anything beyond the version string (no build metadata, no changelog).
package version
