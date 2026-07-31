package session

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/mirageglobe/inari/internal/provider"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusDone     Status = "done"
	StatusDetached Status = "detached"
)

type Tier string

const (
	TierSensor  Tier = "sensor"
	TierWorker  Tier = "worker"
	TierThinker Tier = "thinker"
)

// Session is a named chat context. It is the primary entity in inari.
// Chat history accumulates regardless of which model is currently assigned —
// models can be loaded and unloaded freely while the conversation persists.
// Model is empty when no model is attached; chat is blocked until one is assigned.
// SystemPrompt is mirrored as Messages[0] (role:"system") so it is sent to Ollama
// exactly once per conversation rather than re-prepended on every request.
// CWD is the working directory injected as filesystem context at session creation;
// empty means no filesystem context was provided.
type Session struct {
	mu           sync.Mutex         // protects Messages
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Model        string             `json:"model"`
	Tier         Tier               `json:"tier"`
	Status       Status             `json:"status"`
	Messages     []provider.Message `json:"messages"`
	SystemPrompt string             `json:"system_prompt,omitempty"`
	CWD          string             `json:"cwd,omitempty"`
	// Tags are free-form labels for grouping and filtering sessions in the UI.
	Tags []string `json:"tags,omitempty"`
	// Role is the session's task role ("general"/"coding"); drives the default
	// recommended model. empty means no role assigned.
	Role string `json:"role,omitempty"`
	// NumCtxOverride, when > 0, is the num_ctx the daemon requests for this session
	// instead of the model-derived default; 0 means "use the computed default".
	NumCtxOverride int       `json:"num_ctx_override,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// defaultSystemPrompt is injected into every new session so responses stay concise out of the box.
// users can override it per-session from the describe view.
const defaultSystemPrompt = "keep all responses concise and short. respond in plain text only — no markdown, no bullet points, no bold or italic formatting."

// New returns a new session with a random ID, the given display name, and the default system prompt.
func New(name string) *Session {
	return &Session{
		ID:           newID(),
		Name:         name,
		Status:       StatusPending,
		SystemPrompt: defaultSystemPrompt,
		Messages:     []provider.Message{{Role: "system", Content: defaultSystemPrompt}},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// newID generates an 8-hex-char session ID. 4 bytes gives 4 billion possible values —
// more than sufficient for a local daemon managing O(10) sessions.
func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SetSystemPrompt updates the stored SystemPrompt field and keeps Messages[0]
// (role:"system") in sync so Ollama receives the prompt as part of the natural
// history rather than having it re-injected on every request.
// Passing an empty string removes the system message entirely.
func (s *Session) SetSystemPrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SystemPrompt = prompt
	hasSystem := len(s.Messages) > 0 && s.Messages[0].Role == "system"
	if prompt == "" {
		if hasSystem {
			s.Messages = s.Messages[1:]
		}
		return
	}
	if hasSystem {
		s.Messages[0].Content = prompt
	} else {
		s.Messages = append([]provider.Message{{Role: "system", Content: prompt}}, s.Messages...)
	}
	s.UpdatedAt = time.Now()
}

// AppendMessage appends msg to the session history under the session lock.
func (s *Session) AppendMessage(msg provider.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// RemoveLast drops the final message under the session lock; a no-op on an empty
// history. mirrors AppendMessage so callers never mutate Messages without the lock
// (a direct slice truncation races with ChatHistory/persist readers on other RPCs).
func (s *Session) RemoveLast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Messages) == 0 {
		return
	}
	s.Messages = s.Messages[:len(s.Messages)-1]
	s.UpdatedAt = time.Now()
}

// ToggleTag adds tag if absent or removes it if present, keeping Tags sorted and
// deduped. it reports whether the tag is present after the toggle (true = added).
// an empty tag is a no-op returning false.
func (s *Session) ToggleTag(tag string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tag == "" {
		return false
	}
	for i, t := range s.Tags {
		if t == tag {
			s.Tags = append(s.Tags[:i], s.Tags[i+1:]...)
			s.UpdatedAt = time.Now()
			return false
		}
	}
	s.Tags = append(s.Tags, tag)
	sort.Strings(s.Tags)
	s.UpdatedAt = time.Now()
	return true
}

// SetRole sets the session's task role (e.g. "general"/"coding"); "" clears it.
func (s *Session) SetRole(role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Role = role
	s.UpdatedAt = time.Now()
}

// SetNumCtx sets the per-session num_ctx override; 0 clears it (revert to the
// model-derived default). negative values are clamped to 0.
func (s *Session) SetNumCtx(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 {
		n = 0
	}
	s.NumCtxOverride = n
	s.UpdatedAt = time.Now()
}

// ReplaceWithSummary clears user/assistant messages and inserts summary as a single
// assistant message. the system prompt is retained so model behaviour is unchanged.
func (s *Session) ReplaceWithSummary(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.Messages[:0:0]
	for _, m := range s.Messages {
		if m.Role == "system" {
			kept = append(kept, m)
			break
		}
	}
	kept = append(kept, provider.Message{Role: "assistant", Content: summary})
	s.Messages = kept
	s.UpdatedAt = time.Now()
}

// ClearHistory removes all user and assistant messages, retaining only the system prompt.
// the system prompt stays so the model's behaviour is preserved after a clear.
func (s *Session) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.Messages[:0:0]
	for _, m := range s.Messages {
		if m.Role == "system" {
			kept = append(kept, m)
			break
		}
	}
	s.Messages = kept
	s.UpdatedAt = time.Now()
}

// ChatHistory returns a snapshot of the message history for sending to Ollama.
// A copy is returned so the caller can hold the slice safely while new messages are appended.
func (s *Session) ChatHistory() []provider.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]provider.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}
