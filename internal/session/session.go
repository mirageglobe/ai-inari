// Package session tracks the lifecycle of every model session owned by inarid.
// Sessions survive fox detaching and can be reconnected by ID.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/mirageglobe/ai-inari/internal/ollama"
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

// Session is a named chat context. Name and chat history live here so they
// survive fox detach/reattach cycles. Model is empty until the user assigns one.
type Session struct {
	mu        sync.Mutex       // protects Messages
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Model     string           `json:"model"`
	Tier      Tier             `json:"tier"`
	Status    Status           `json:"status"`
	Messages  []ollama.Message `json:"messages"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// New returns a new session with a random ID and the given display name.
func New(name string) *Session {
	return &Session{
		ID:        newID(),
		Name:      name,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// AppendMessage appends msg to the session history under the session lock.
func (s *Session) AppendMessage(msg ollama.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// ChatHistory returns a snapshot of the message history for sending to Ollama.
func (s *Session) ChatHistory() []ollama.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ollama.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// Store holds all active and background sessions.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

func (s *Store) Add(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out
}
