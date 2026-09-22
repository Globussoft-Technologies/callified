package recording

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCallOutcome(t *testing.T) {
	tests := []struct {
		name              string
		appointmentBooked bool
		modelOutcome      string
		want              string
	}{
		{name: "confirmed appointment wins", appointmentBooked: true, modelOutcome: callOutcomeNotInterested, want: callOutcomeAppointmentBooked},
		{name: "clear rejection", modelOutcome: callOutcomeNotInterested, want: callOutcomeNotInterested},
		{name: "unfinished call", modelOutcome: callOutcomePending, want: callOutcomePending},
		{name: "missing outcome defaults safely", want: callOutcomePending},
		{name: "unexpected outcome defaults safely", modelOutcome: "no_appointment", want: callOutcomePending},
		{name: "normalizes whitespace and case", modelOutcome: "  NOT_INTERESTED  ", want: callOutcomeNotInterested},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analysis{
				AppointmentBooked: tt.appointmentBooked,
				CallOutcome:       tt.modelOutcome,
			}
			normalizeCallOutcome(a)
			require.Equal(t, tt.want, a.CallOutcome)
		})
	}
}
