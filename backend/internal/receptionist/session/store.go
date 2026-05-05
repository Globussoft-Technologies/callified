// Package session provides per-caller state with TTL-based eviction.
//
// Replace SessionStore's map with a Redis-backed implementation for
// multi-instance deployments — the public method set stays the same.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/globussoft/callified-backend/internal/receptionist/models"
)

// Turn is one entry in the session transcript.
type Turn struct {
	Role string    // "user" | "assistant" | "system"
	Text string
	TS   time.Time
}

// Session is one caller's state. All fields are mutable under Store's lock.
type Session struct {
	ID         string
	CallerID   string
	Language   string
	State      models.ConversationState
	Slots      map[string]any
	Transcript []Turn
	CreatedAt  time.Time
	LastActive time.Time
}

// Touch updates the last-active timestamp; call after any mutation.
func (s *Session) Touch() {
	s.LastActive = time.Now()
}

// Append records one transcript turn and touches the session.
func (s *Session) Append(role, text string) {
	s.Transcript = append(s.Transcript, Turn{Role: role, Text: text, TS: time.Now()})
	s.Touch()
}

// Store is an in-memory session store with TTL eviction.
type Store struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]*Session
}

// New constructs a Store with the given TTL.
func New(ttl time.Duration) *Store {
	return &Store{ttl: ttl, m: make(map[string]*Session)}
}

// Create makes a new session and returns it.
func (s *Store) Create(callerID, language string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if language == "" {
		language = "en-US"
	}
	now := time.Now()
	sess := &Session{
		ID:         randID(),
		CallerID:   callerID,
		Language:   language,
		State:      models.StateGreeting,
		Slots:      map[string]any{},
		CreatedAt:  now,
		LastActive: now,
	}
	s.m[sess.ID] = sess
	s.sweepLocked()
	return sess
}

// Get returns a session by id, or nil if missing/expired.
func (s *Store) Get(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	return s.m[id]
}

// End removes the session and marks it ended. Returns the (final) session.
func (s *Store) End(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[id]
	if !ok {
		return nil
	}
	delete(s.m, id)
	sess.State = models.StateEnded
	return sess
}

// sweepLocked drops sessions inactive past the TTL. Caller holds the lock.
func (s *Store) sweepLocked() {
	cutoff := time.Now().Add(-s.ttl)
	for id, sess := range s.m {
		if sess.LastActive.Before(cutoff) {
			delete(s.m, id)
		}
	}
}

// randID generates a 32-hex-char session id (~128 bits of entropy).
func randID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is catastrophic; the runtime will be
		// unusable anyway. Fall back to a timestamp so we don't return
		// duplicates in the bizarre edge case.
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}
