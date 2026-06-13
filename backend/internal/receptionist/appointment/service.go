// Package appointment is an in-memory booking store.
//
// In production, replace Service's map with Postgres or your EHR
// integration. The exported method set is the seam — keep callers
// depending on Service, not on the dict.
package appointment

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DoctorEntry represents one row of the clinic's doctor roster.
type DoctorEntry struct {
	Name      string `json:"name"`
	Specialty string `json:"specialty"`
}

// Roster is the mock doctor list. Replace with a directory-service call
// in production.
var Roster = []DoctorEntry{
	{Name: "Dr. John", Specialty: "General Practice"},
	{Name: "Dr. Mike", Specialty: "Pediatrics"},
	{Name: "Dr. Emma", Specialty: "Cardiology"},
	{Name: "Dr. Elina", Specialty: "Dermatology"},
}

// Record is one booked appointment.
type Record struct {
	ID           string
	PatientName  string
	Doctor       string
	ScheduledFor time.Time
	Status       string
	CreatedAt    time.Time
}

// Errors surface specific failure modes so callers can react
// appropriately (re-prompt for doctor, re-prompt for time, etc.).
var (
	ErrNotFound        = errors.New("no active appointment with that id")
	ErrSlotUnavailable = errors.New("that slot is already booked")
	ErrUnknownDoctor   = errors.New("no doctor matched")
	ErrBadTime         = errors.New("could not parse time")
)

// Service is a thread-safe in-memory store.
type Service struct {
	mu sync.Mutex
	m  map[string]*Record
}

// New returns a fresh service.
func New() *Service {
	return &Service{m: make(map[string]*Record)}
}

// AvailableDoctors returns the static roster.
func (s *Service) AvailableDoctors() []DoctorEntry {
	out := make([]DoctorEntry, len(Roster))
	copy(out, Roster)
	return out
}

// resolveDoctor maps free-form caller input to a canonical roster name.
func (s *Service) resolveDoctor(query string) (string, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return "", fmt.Errorf("%w: no doctor specified", ErrUnknownDoctor)
	}
	q = regexp.MustCompile(`^(?:dr\.?|doctor)\s+`).ReplaceAllString(q, "")

	for _, d := range Roster {
		nameL := strings.ToLower(d.Name)
		specL := strings.ToLower(d.Specialty)
		if strings.Contains(nameL, q) || strings.Contains(specL, q) {
			return d.Name, nil
		}
		parts := strings.Fields(d.Name)
		last := strings.ToLower(parts[len(parts)-1])
		if q == last {
			return d.Name, nil
		}
	}

	aliases := map[string]string{
		"pediatrician":         "Dr. Mike",
		"kids doctor":          "Dr. Mike",
		"cardiologist":         "Dr. Emma",
		"heart doctor":         "Dr. Emma",
		"dermatologist":        "Dr. Elina",
		"skin doctor":          "Dr. Elina",
		"gp":                   "Dr. John",
		"general practitioner": "Dr. John",
		"family doctor":        "Dr. John",
	}
	if name, ok := aliases[q]; ok {
		return name, nil
	}
	return "", fmt.Errorf("%w: '%s'", ErrUnknownDoctor, query)
}

// ParseWhen does best-effort parsing of caller-supplied date+time text.
// Returns ErrBadTime on patently invalid input.
func ParseWhen(dateText, timeText string) (time.Time, error) {
	now := time.Now().UTC().Truncate(time.Second)
	target := now.Add(24 * time.Hour)
	target = time.Date(target.Year(), target.Month(), target.Day(), 10, 0, 0, 0, time.UTC)

	if dateText != "" {
		d := strings.ToLower(strings.TrimSpace(dateText))
		switch d {
		case "today":
			target = time.Date(now.Year(), now.Month(), now.Day(),
				target.Hour(), target.Minute(), 0, 0, time.UTC)
		case "tomorrow":
			tt := now.Add(24 * time.Hour)
			target = time.Date(tt.Year(), tt.Month(), tt.Day(),
				target.Hour(), target.Minute(), 0, 0, time.UTC)
		default:
			weekdays := map[string]time.Weekday{
				"monday": time.Monday, "tuesday": time.Tuesday,
				"wednesday": time.Wednesday, "thursday": time.Thursday,
				"friday": time.Friday, "saturday": time.Saturday,
				"sunday": time.Sunday,
			}
			if wd, ok := weekdays[d]; ok {
				delta := (int(wd) - int(now.Weekday()) + 7) % 7
				if delta == 0 {
					delta = 7
				}
				tt := now.AddDate(0, 0, delta)
				target = time.Date(tt.Year(), tt.Month(), tt.Day(),
					target.Hour(), target.Minute(), 0, 0, time.UTC)
			} else {
				// ISO / m/d/y
				layouts := []string{"2006-01-02", "01/02/2006", "1/2/2006",
					"01/02/06", "1/2/06", "01/02", "1/2"}
				parsed := false
				for _, layout := range layouts {
					if t, err := time.Parse(layout, d); err == nil {
						y := t.Year()
						if y == 0 {
							y = now.Year()
						}
						target = time.Date(y, t.Month(), t.Day(),
							target.Hour(), target.Minute(), 0, 0, time.UTC)
						parsed = true
						break
					}
				}
				if !parsed {
					// Soft-fail: keep default target. Date wasn't parseable
					// but we don't need to error out — caller can retry.
				}
			}
		}
	}

	if timeText != "" {
		t := strings.ToLower(strings.ReplaceAll(timeText, " ", ""))
		re := regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)?$`)
		m := re.FindStringSubmatch(t)
		if m == nil {
			return time.Time{}, fmt.Errorf("%w: '%s'", ErrBadTime, timeText)
		}
		hour, _ := strconv.Atoi(m[1])
		minute := 0
		if m[2] != "" {
			minute, _ = strconv.Atoi(m[2])
		}
		switch m[3] {
		case "pm":
			if hour < 12 {
				hour += 12
			}
		case "am":
			if hour == 12 {
				hour = 0
			}
		}
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return time.Time{}, fmt.Errorf("%w: '%s'", ErrBadTime, timeText)
		}
		target = time.Date(target.Year(), target.Month(), target.Day(),
			hour, minute, 0, 0, time.UTC)
	}

	return target, nil
}

// Book creates a new appointment after resolving doctor and parsing time.
func (s *Service) Book(patientName, doctorQuery, dateText, timeText string) (*Record, error) {
	doctor, err := s.resolveDoctor(doctorQuery)
	if err != nil {
		return nil, err
	}
	when, err := ParseWhen(dateText, timeText)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.m {
		if r.Status == "confirmed" && r.Doctor == doctor && r.ScheduledFor.Equal(when) {
			return nil, fmt.Errorf("%w: %s at %s",
				ErrSlotUnavailable, doctor, when.Format("Monday 3:04 PM"))
		}
	}

	rec := &Record{
		ID:           "APT-" + randHex(3),
		PatientName:  patientName,
		Doctor:       doctor,
		ScheduledFor: when,
		Status:       "confirmed",
		CreatedAt:    time.Now().UTC(),
	}
	s.m[rec.ID] = rec
	return rec, nil
}

// Reschedule updates an existing booking's time.
func (s *Service) Reschedule(id, dateText, timeText string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.m[strings.ToUpper(id)]
	if !ok || rec.Status == "cancelled" {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	when, err := ParseWhen(dateText, timeText)
	if err != nil {
		return nil, err
	}
	for _, other := range s.m {
		if other.ID != rec.ID && other.Status == "confirmed" &&
			other.Doctor == rec.Doctor && other.ScheduledFor.Equal(when) {
			return nil, fmt.Errorf("%w: %s at %s",
				ErrSlotUnavailable, rec.Doctor, when.Format("Monday 3:04 PM"))
		}
	}
	rec.ScheduledFor = when
	return rec, nil
}

// Cancel marks a booking cancelled. Idempotent.
func (s *Service) Cancel(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.m[strings.ToUpper(id)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if rec.Status == "cancelled" {
		return rec, nil
	}
	rec.Status = "cancelled"
	return rec, nil
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback — unique enough for non-cryptographic ID purposes.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return strings.ToUpper(hex.EncodeToString(b))
}
