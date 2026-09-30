package wshandler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateAppointmentSlotAcceptsOnlyFutureInstant(t *testing.T) {
	location, err := time.LoadLocation("Asia/Kolkata")
	assert.NoError(t, err)
	now := time.Date(2026, time.September, 30, 10, 0, 0, 0, location)

	assert.Equal(t, appointmentSlotValid, validateAppointmentSlot("today", "11:00", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotValid, validateAppointmentSlot("tomorrow", "eleven AM", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotValid, validateAppointmentSlot("2026-10-02", "15:30", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotValid, validateAppointmentSlot("September thirtieth", "11 AM", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotPast, validateAppointmentSlot("today", "09:30", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotPast, validateAppointmentSlot("2026-09-29", "18:00", "Asia/Kolkata", now))
}

func TestValidateAppointmentSlotRejectsMissingOrAmbiguousValues(t *testing.T) {
	location, err := time.LoadLocation("Asia/Kolkata")
	assert.NoError(t, err)
	now := time.Date(2026, time.September, 30, 10, 0, 0, 0, location)

	assert.Equal(t, appointmentSlotMissingDate, validateAppointmentSlot("", "11:00", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotMissingTime, validateAppointmentSlot("tomorrow", "", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotInvalid, validateAppointmentSlot("some day", "morning", "Asia/Kolkata", now))
	assert.Equal(t, appointmentSlotInvalid, validateAppointmentSlot("tomorrow", "three", "Asia/Kolkata", now))
}
