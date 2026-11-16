package realtime

import (
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestToolActionsRejectInvalidRequestsAndReportActualResults(t *testing.T) {
	for _, tc := range []struct {
		name, tool, outcome, date, clock, want string
		callback, allow, invoked               bool
	}{
		{name: "interview complete", tool: "complete_call", outcome: "conversation_completed", callback: true, allow: true, invoked: true, want: "accepted:"},
		{name: "unknown outcome", tool: "complete_call", outcome: "invented", callback: true, want: "rejected:"},
		{name: "missing outcome", tool: "complete_call", callback: true, want: "rejected:"},
		{name: "missing appointment time", tool: "complete_call", outcome: "appointment_booked", date: "2027-01-01", callback: true, want: "rejected:"},
		{name: "interview cannot smuggle appointment", tool: "complete_call", outcome: "conversation_completed", date: "2027-01-01", callback: true, want: "rejected:"},
		{name: "missing completion handler", tool: "complete_call", outcome: "conversation_completed", want: "rejected:"},
		{name: "backend rejects appointment", tool: "complete_call", outcome: "appointment_booked", date: "2027-01-01", clock: "10:00", callback: true, invoked: true, want: "rejected:"},
		{name: "unknown tool", tool: "invented", want: "rejected:"},
		{name: "missing knowledge handler", tool: "search_product_knowledge", want: "No additional verified product information"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replies := make(chan map[string]any, 1)
			conn := liveTestConnection(t, func(server *websocket.Conn) {
				_ = server.SetReadDeadline(time.Now().Add(3 * time.Second))
				var reply map[string]any
				if server.ReadJSON(&reply) == nil {
					replies <- reply
				}
			})
			invoked := false
			cb := Callbacks{}
			if tc.callback {
				cb.OnCompleteCall = func(request CompleteCallRequest) bool {
					invoked = true
					require.Equal(t, tc.outcome, request.Outcome)
					return tc.allow
				}
			}
			client := New(Config{}, cb)
			client.handleToolCall(conn, map[string]any{"functionCalls": []any{
				map[string]any{"id": "action-1", "name": tc.tool, "args": map[string]any{
					"outcome": tc.outcome, "appointment_date": tc.date, "appointment_time": tc.clock,
				}},
			}})
			require.Equal(t, tc.invoked, invoked)
			select {
			case reply := <-replies:
				responses := reply["toolResponse"].(map[string]any)["functionResponses"].([]any)
				response := responses[0].(map[string]any)
				require.Equal(t, "action-1", response["id"])
				result := response["response"].(map[string]any)["result"].(string)
				require.True(t, strings.Contains(result, tc.want), "unexpected tool result: %s", result)
			case <-time.After(4 * time.Second):
				t.Fatal("no tool response received")
			}
		})
	}
}
