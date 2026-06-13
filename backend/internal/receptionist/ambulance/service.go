// Package ambulance is a MOCK ambulance dispatch service.
//
// In production, replace Service with an integration into your local
// EMS partner's API. The exported method set is the seam — the
// conversation manager has no other coupling to dispatch logic.
//
// Important: even with this enabled, the conversation always tells the
// caller to dial 911 first. Auto-dispatch is supplementary, not a
// replacement for trained 911 dispatchers.
package ambulance

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"
)

// vehicle is one paramedic crew assignment. Replace with a fleet API.
type vehicle struct {
	ID   string
	Crew string
}

var fleet = []vehicle{
	{ID: "EMS-12", Crew: "Paramedic Lewis"},
	{ID: "EMS-17", Crew: "Paramedic Rivera"},
	{ID: "EMS-23", Crew: "Paramedic Okafor"},
	{ID: "EMS-31", Crew: "Paramedic Chen"},
	{ID: "EMS-44", Crew: "Paramedic Müller"},
}

// Dispatch is one ambulance booking record.
type Dispatch struct {
	ID             string
	SessionID      string
	CallerID       string
	PatientName    string
	Location       string
	MatchedPhrase  string
	TranscriptTail string
	VehicleID      string
	CrewLead       string
	ETAMinutes     int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Service is a thread-safe in-memory dispatch store.
type Service struct {
	mu        sync.Mutex
	store     map[string]*Dispatch // by dispatch id
	bySession map[string]string    // session id -> dispatch id
}

// New constructs an empty Service.
func New() *Service {
	return &Service{
		store:     make(map[string]*Dispatch),
		bySession: make(map[string]string),
	}
}

// DispatchInput collects all fields needed to create a dispatch.
type DispatchInput struct {
	SessionID      string
	CallerID       string
	PatientName    string
	MatchedPhrase  string
	TranscriptTail string
	Location       string
}

// Dispatch creates-or-returns the active dispatch for this session.
//
// Idempotent: subsequent emergency utterances during the same call
// return the same record rather than spawning duplicates.
func (s *Service) Dispatch(in DispatchInput) *Dispatch {
	s.mu.Lock()
	defer s.mu.Unlock()

	if did, ok := s.bySession[in.SessionID]; ok {
		rec := s.store[did]
		rec.TranscriptTail = clip(in.TranscriptTail, 280)
		if in.MatchedPhrase != "" && rec.MatchedPhrase == "" {
			rec.MatchedPhrase = in.MatchedPhrase
		}
		rec.UpdatedAt = time.Now().UTC()
		return rec
	}

	v := pickVehicle()
	rec := &Dispatch{
		ID:             "AMB-" + strings.ToUpper(randHex(3)),
		SessionID:      in.SessionID,
		CallerID:       in.CallerID,
		PatientName:    in.PatientName,
		Location:       in.Location,
		MatchedPhrase:  in.MatchedPhrase,
		TranscriptTail: clip(in.TranscriptTail, 280),
		VehicleID:      v.ID,
		CrewLead:       v.Crew,
		ETAMinutes:     6 + randInt(7), // 6-12 min
		Status:         "dispatched",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	s.store[rec.ID] = rec
	s.bySession[in.SessionID] = rec.ID
	log.Printf("AMBULANCE DISPATCH %s vehicle=%s crew=%q eta=%dmin session=%s phrase=%q",
		rec.ID, rec.VehicleID, rec.CrewLead, rec.ETAMinutes,
		rec.SessionID, rec.MatchedPhrase)
	return rec
}

// UpdateLocation sets the address on an existing dispatch.
func (s *Service) UpdateLocation(id, location string) *Dispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.store[strings.ToUpper(id)]
	if !ok {
		return nil
	}
	rec.Location = location
	rec.UpdatedAt = time.Now().UTC()
	log.Printf("AMBULANCE %s location updated to %q", rec.ID, location)
	return rec
}

// GetForSession returns the active dispatch for a session, if any.
func (s *Service) GetForSession(sessionID string) *Dispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	if did, ok := s.bySession[sessionID]; ok {
		return s.store[did]
	}
	return nil
}

// --- helpers ----------------------------------------------------------

func pickVehicle() vehicle {
	return fleet[randInt(len(fleet))]
}

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return int(time.Now().UnixNano()) % n
	}
	return int(v.Int64())
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
