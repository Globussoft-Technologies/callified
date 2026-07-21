package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in       string
		expected string
	}{
		{"9876543210", "9876543210"},       // mobile
		{"+919876543210", "9876543210"},    // mobile with +91
		{"919876543210", "9876543210"},     // mobile with 91
		{"09876543210", "9876543210"},      // mobile with leading 0
		{"01112345678", "1112345678"},      // delhi landline
		{"+91-11-1234-5678", "1112345678"}, // formatted landline
		{"022-1234-5678", "2212345678"},    // mumbai landline
		{"080-12345678", "8012345678"},     // bangalore landline
		{"+91 98765 43210", "9876543210"},  // spaced mobile
		{"", ""},                           // empty
		{"123", ""},                        // too short
		{"1234567890123", ""},              // too long
	}

	for _, c := range cases {
		got := normalizePhone(c.in)
		if got != c.expected {
			t.Errorf("normalizePhone(%q) = %q, want %q", c.in, got, c.expected)
		}
	}
}

func TestIsValidPhone(t *testing.T) {
	valid := []string{
		"9876543210",
		"+919876543210",
		"01112345678",
		"+91-11-1234-5678",
		"022-1234-5678",
	}
	invalid := []string{
		"",
		"123",
		"abcdefghij",
		"1234567890123",
	}
	for _, p := range valid {
		if !isValidPhone(p) {
			t.Errorf("isValidPhone(%q) = false, want true", p)
		}
	}
	for _, p := range invalid {
		if isValidPhone(p) {
			t.Errorf("isValidPhone(%q) = true, want false", p)
		}
	}
}

func TestNameHasAllowedChars(t *testing.T) {
	valid := []string{"sri", "Sri Kumar", "A. Kumar", "O'Neil", "Lead 1"}
	invalid := []string{"", "123", "@@@"}

	for _, name := range valid {
		if !nameHasAllowedChars(name) {
			t.Errorf("nameHasAllowedChars(%q) = false, want true", name)
		}
	}
	for _, name := range invalid {
		if nameHasAllowedChars(name) {
			t.Errorf("nameHasAllowedChars(%q) = true, want false", name)
		}
	}
}

func TestIsDuplicateEntryError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"mysql duplicate", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, true},
		{"wrapped mysql duplicate", fmt.Errorf("insert lead: %w", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}), true},
		{"unique constraint text", errors.New("UNIQUE constraint failed: leads.phone"), true},
		{"other mysql error", &mysql.MySQLError{Number: 1048, Message: "Column cannot be null"}, false},
		{"other error", errors.New("connection refused"), false},
	}

	for _, c := range cases {
		if got := isDuplicateEntryError(c.err); got != c.want {
			t.Errorf("%s: isDuplicateEntryError() = %v, want %v", c.name, got, c.want)
		}
	}
}
