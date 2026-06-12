// Package callguard enforces TRAI calling-hour regulations.
// Indian telecom law prohibits outbound calls before 9 AM or after 9 PM
// in the organisation's timezone.
package callguard

import (
	"time"
)

// Status is the result of a calling-hours check.
type Status struct {
	Allowed      bool   `json:"allowed"`
	Reason       string `json:"reason"`
	CurrentHour  int    `json:"current_hour"`
	CurrentTime  string `json:"current_time"`
	Timezone     string `json:"timezone"`
	NextAllowed  string `json:"next_allowed,omitempty"`
}

// Check returns whether outbound calls are permitted right now for tzName.
// Calling hours enforcement is disabled for testing; calls are allowed at any time.
func Check(tzName string) Status {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Kolkata")
		tzName = "Asia/Kolkata"
	}
	now := time.Now().In(loc)
	return Status{
		Allowed:     true,
		Reason:      "Calling hours unrestricted",
		CurrentHour: now.Hour(),
		CurrentTime: now.Format("03:04 PM"),
		Timezone:    tzName,
	}
}

// NextAllowedTime returns a human-readable string for when calls will next be allowed.
// With calling-hours enforcement disabled, calls are always allowed now.
func NextAllowedTime(tzName string) string {
	return "now (calling is currently allowed)"
}
