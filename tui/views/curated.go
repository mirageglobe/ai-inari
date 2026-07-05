// curated model recommendations for the model selector.
// this file owns the CuratedModel table (kept in sync with SPEC.md §6.1),
// hardware-tier detection, and the filter that picks recommendations not
// already pulled locally. it does NOT own model listing/loading - that
// stays in selector.go, which reads from CuratedModels via RecommendedFor.

package views

// CuratedModel is one entry from SPEC.md's §6.1 Ollama Model Curation table.
type CuratedModel struct {
	TierGB int
	Role   string // "general" or "coding"
	Model  string
	Size   string
	Notes  string
}

// CuratedModels mirrors SPEC.md §6.1. update both together.
var CuratedModels = []CuratedModel{
	{32, "general", "gemma4:27b", "~15gb", "google moe; near-frontier chat and review"},
	{16, "general", "phi-4:14b", "~8gb", "microsoft; strong multi-file reasoning"},
	{8, "general", "gemma4:e4b", "~2.7gb", "4.5b effective; fast routing and quick queries"},
	{4, "general", "llama3.2:3b", "~2gb", "meta; best chat and reasoning within 4gb"},

	{32, "coding", "qwen3.6:27b-coding-nvfp4", "~18gb", "alibaba; near-frontier generation and review"},
	{16, "coding", "deepseek-r1:14b", "~9gb", "r1-671b distil; strong coding and reasoning"},
	{8, "coding", "deepseek-r1:8b", "~5gb", "r1-671b distil; fits 8gb; coding+reasoning"},
	{4, "coding", "llama3.2:3b", "~2gb", "meta; best within 4gb budget"},
}

// curatedTiers are the hardware tiers from SPEC.md §6.1, largest first.
var curatedTiers = []int{32, 16, 8, 4}

// DetectTier buckets totalBytes into the nearest §6.1 tier at or below it,
// flooring at the smallest tier (4gb) for anything below that.
func DetectTier(totalBytes uint64) int {
	totalGB := int(totalBytes / (1 << 30))
	for _, t := range curatedTiers {
		if totalGB >= t {
			return t
		}
	}
	return curatedTiers[len(curatedTiers)-1]
}

// RecommendedFor returns the curated models for tierGB that are not already
// present in available (matched by exact model name).
func RecommendedFor(tierGB int, available []string) []CuratedModel {
	have := make(map[string]bool, len(available))
	for _, a := range available {
		have[a] = true
	}
	var out []CuratedModel
	for _, c := range CuratedModels {
		if c.TierGB == tierGB && !have[c.Model] {
			out = append(out, c)
		}
	}
	return out
}
