// Package session tracks the lifecycle of every model session owned by inarid.
// Sessions survive fox detaching and can be reconnected by ID.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

// Session is a named chat context. It is the primary entity in ai-inari.
// Chat history accumulates regardless of which model is currently assigned —
// models can be loaded and unloaded freely while the conversation persists.
// Model is empty when no model is attached; chat is blocked until one is assigned.
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

// newID generates an 8-hex-char session ID. 4 bytes gives 4 billion possible values —
// more than sufficient for a local daemon managing O(10) sessions.
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
// A copy is returned so the caller can hold the slice safely while new messages are appended.
func (s *Session) ChatHistory() []ollama.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ollama.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// Store holds all active and background sessions.
// RWMutex allows concurrent reads (Get, List) while serialising writes (Add, Remove).
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	dir      string // empty means no persistence
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

// NewPersistentStore creates a Store backed by JSON files in dir.
// Existing session files are loaded on startup so sessions survive daemon restarts.
func NewPersistentStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("session store: create dir: %w", err)
	}
	s := &Store{sessions: make(map[string]*Session), dir: dir}
	return s, s.loadAll()
}

// loadAll reads all *.json files from dir into the in-memory map.
func (s *Store) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("session store: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			return fmt.Errorf("session store: load %s: %w", e.Name(), err)
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return fmt.Errorf("session store: parse %s: %w", e.Name(), err)
		}
		s.sessions[sess.ID] = &sess
	}
	return nil
}

// persist writes sess to dir/<id>.json via a tmp+rename so readers never see a partial file.
func (s *Store) persist(sess *Session) {
	if s.dir == "" {
		return
	}
	sess.mu.Lock()
	data, err := json.Marshal(sess)
	sess.mu.Unlock()
	if err != nil {
		log.Printf("session persist: marshal %s: %v", sess.ID, err)
		return
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("session persist: write %s: %v", sess.ID, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("session persist: rename %s: %v", sess.ID, err)
	}
}

// Persist saves the session with the given ID to disk.
// Called by the server after in-place mutations (assign, unassign, chat).
func (s *Store) Persist(id string) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if ok {
		s.persist(sess)
	}
}

func (s *Store) Add(sess *Session) {
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	s.persist(sess)
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	if s.dir != "" {
		os.Remove(filepath.Join(s.dir, id+".json")) // best-effort
	}
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
