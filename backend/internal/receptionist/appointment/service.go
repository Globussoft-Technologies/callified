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

// ErrBadDate signals that a date string was given but couldn't be parsed.
// Surfaced as a distinct error so callers can re-prompt with a date-specific
// hint instead of the generic time hint.
var ErrBadDate = errors.New("could not parse date")

// parseDateText turns caller-supplied date phrasing into a Y/M/D triplet.
// Returns ok=false when the input is non-empty but not parseable — callers
// should treat that as a hard error, NOT silently default. (Silent defaults
// caused "27th May 2026" to book tomorrow at 10 AM.)
func parseDateText(dateText string, now time.Time) (year int, month time.Month, day int, ok bool) {
	d := strings.ToLower(strings.TrimSpace(dateText))
	if d == "" {
		// Empty input means "use the default" — caller decides what that is.
		return 0, 0, 0, false
	}
	switch d {
	case "today":
		return now.Year(), now.Month(), now.Day(), true
	case "tomorrow":
		tt := now.Add(24 * time.Hour)
		return tt.Year(), tt.Month(), tt.Day(), true
	}
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
		return tt.Year(), tt.Month(), tt.Day(), true
	}

	// Strip ordinal suffixes ("27th" → "27") and commas so layouts can match.
	stripped := regexp.MustCompile(`(\d+)(st|nd|rd|th)\b`).ReplaceAllString(d, "$1")
	stripped = strings.ReplaceAll(stripped, ",", "")
	stripped = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(stripped, " "))
	stripped = strings.TrimPrefix(stripped, "of ")
	// Normalize "27 of may 2026" → "27 may 2026"
	stripped = regexp.MustCompile(`\s+of\s+`).ReplaceAllString(stripped, " ")

	layouts := []string{
		"2006-01-02",
		"01/02/2006", "1/2/2006", "01/02/06", "1/2/06",
		"02/01/2006", "2/1/2006", // d/m/y for ambiguous slashes (after m/d/y above)
		"01/02", "1/2",
		"2 January 2006", "2 Jan 2006",
		"January 2 2006", "Jan 2 2006",
		"2 January", "2 Jan",
		"January 2", "Jan 2",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, stripped); err == nil {
			y := t.Year()
			if y == 0 {
				y = now.Year()
				// If the parsed month/day already passed this year and no
				// year was supplied, roll to next year so we don't book in
				// the past.
				cand := time.Date(y, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
				today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				if cand.Before(today) {
					y++
				}
			}
			return y, t.Month(), t.Day(), true
		}
	}
	return 0, 0, 0, false
}

// parseTimeText turns caller-supplied time phrasing into an (hour, minute)
// pair. Handles "6am", "6:30 pm", "6 o'clock", "6 in the morning",
// "morning 6", "evening 7", "18:30". Returns ok=false on unparseable input.
func parseTimeText(timeText string) (hour, minute int, ok bool) {
	t := strings.ToLower(strings.TrimSpace(timeText))
	if t == "" {
		return 0, 0, false
	}

	// Detect part-of-day word so we can imply AM/PM for bare hours.
	partOfDay := ""
	for _, w := range []string{"morning", "afternoon", "evening", "night"} {
		if strings.Contains(t, w) {
			partOfDay = w
			break
		}
	}

	// Pull the numeric "H" or "H:MM" out of the string. We accept the first
	// such match — input has already passed the intent regex, so it's not
	// arbitrary text.
	num := regexp.MustCompile(`(\d{1,2})(?::(\d{2}))?`).FindStringSubmatch(t)
	if num == nil {
		return 0, 0, false
	}
	h, _ := strconv.Atoi(num[1])
	m := 0
	if num[2] != "" {
		m, _ = strconv.Atoi(num[2])
	}

	// Apply explicit AM/PM if present, otherwise infer from part of day.
	hasAM := strings.Contains(t, "am") || strings.Contains(t, "a.m")
	hasPM := strings.Contains(t, "pm") || strings.Contains(t, "p.m")
	switch {
	case hasPM:
		if h < 12 {
			h += 12
		}
	case hasAM:
		if h == 12 {
			h = 0
		}
	case partOfDay == "morning":
		if h == 12 {
			h = 0
		}
	case partOfDay == "afternoon", partOfDay == "evening", partOfDay == "night":
		if h < 12 {
			h += 12
		}
	}

	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// ParseWhen does best-effort parsing of caller-supplied date+time text.
// Returns ErrBadDate when a date was given but couldn't be parsed, and
// ErrBadTime when a time was given but couldn't be parsed. When BOTH inputs
// are empty, returns the default of tomorrow at 10 AM. Critically, this
// function never silently snaps to a default when the caller supplied input
// — that caused real bookings to land on the wrong day.
func ParseWhen(dateText, timeText string) (time.Time, error) {
	now := time.Now().UTC().Truncate(time.Second)

	// Default: tomorrow 10:00 — used only when the caller said NOTHING about
	// when. If they did say something we either honor it or error out.
	defaultDay := now.Add(24 * time.Hour)
	year, month, day := defaultDay.Year(), defaultDay.Month(), defaultDay.Day()
	hour, minute := 10, 0

	if strings.TrimSpace(dateText) != "" {
		y, mo, d, ok := parseDateText(dateText, now)
		if !ok {
			return time.Time{}, fmt.Errorf("%w: '%s'", ErrBadDate, dateText)
		}
		year, month, day = y, mo, d
	}

	if strings.TrimSpace(timeText) != "" {
		h, m, ok := parseTimeText(timeText)
		if !ok {
			return time.Time{}, fmt.Errorf("%w: '%s'", ErrBadTime, timeText)
		}
		hour, minute = h, m
	}

	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC), nil
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

// FindByPatient returns confirmed appointments matching patientName
// (case-insensitive substring), newest first. Used by the receptionist
// flow when a caller has forgotten their confirmation number — the bot
// can offer to look it up by name instead of dead-ending the call.
// Returns an empty slice when nothing matches.
func (s *Service) FindByPatient(patientName string) []*Record {
	q := strings.ToLower(strings.TrimSpace(patientName))
	if q == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Record
	for _, r := range s.m {
		if r.Status != "confirmed" {
			continue
		}
		if strings.Contains(strings.ToLower(r.PatientName), q) {
			out = append(out, r)
		}
	}
	// Newest first — the most recent booking is the one a caller is
	// most likely asking about.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// SuggestDoctor returns the closest roster name to a free-form query
// when the query doesn't resolve to a known doctor (e.g. "Hema" → "Dr. Emma").
// Returns "" when no name is similar enough — caller should not invent.
//
// Heuristic: substring overlap or first/last-name initial match. Cheap
// and predictable; avoids pulling in a Levenshtein dependency for what
// is usually a one- or two-character mishearing.
func (s *Service) SuggestDoctor(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	q = regexp.MustCompile(`^(?i)(?:dr\.?|doctor)\s+`).ReplaceAllString(q, "")
	if q == "" {
		return ""
	}
	best := ""
	bestScore := 0
	for _, d := range Roster {
		// Compare against the first name only — the roster has unique
		// firsts and that's what callers say.
		parts := strings.Fields(strings.ToLower(d.Name))
		first := parts[len(parts)-1] // "dr. john" → "john"
		score := commonPrefixLen(q, first)
		// Also accept reversed prefix ("emma" vs "hema" share "ma" suffix).
		if rev := commonSuffixLen(q, first); rev > score {
			score = rev
		}
		// Same first letter is a weak signal — only useful as a tiebreaker.
		if len(q) > 0 && len(first) > 0 && q[0] == first[0] {
			score++
		}
		if score > bestScore {
			best = d.Name
			bestScore = score
		}
	}
	// Require at least 2 chars in common — single-letter matches produce
	// nonsense suggestions. (e.g. "x" → "Dr. John" because they share no
	// letters but score is 1 from the first-letter bonus would be wrong.)
	if bestScore < 2 {
		return ""
	}
	return best
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func commonSuffixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return n
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
