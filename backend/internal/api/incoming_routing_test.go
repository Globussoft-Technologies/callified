package api

import (
	"encoding/xml"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestPlivoForwardConnectsSavedNumberWithoutAI(t *testing.T) {
	w := httptest.NewRecorder()
	require.NoError(t, writePlivoForward(w, "80 6558 9631", "+919876543210", "https://calls.example/action?a=1&b=2", "https://calls.example/event"))
	var response struct {
		Dial struct {
			CallerID       string `xml:"callerId,attr"`
			Action         string `xml:"action,attr"`
			Method         string `xml:"method,attr"`
			CallbackURL    string `xml:"callbackUrl,attr"`
			CallbackMethod string `xml:"callbackMethod,attr"`
			Timeout        int    `xml:"timeout,attr"`
			Number         string `xml:"Number"`
		} `xml:"Dial"`
	}
	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "+918065589631", response.Dial.CallerID)
	require.Equal(t, "+919876543210", response.Dial.Number)
	require.Equal(t, 30, response.Dial.Timeout)
	require.Equal(t, "https://calls.example/action?a=1&b=2", response.Dial.Action)
	require.Equal(t, "POST", response.Dial.Method)
	require.Equal(t, "https://calls.example/event", response.Dial.CallbackURL)
	require.Equal(t, "POST", response.Dial.CallbackMethod)
	require.NotContains(t, w.Body.String(), "Stream")
	require.Contains(t, w.Body.String(), "Hangup")
}
func TestPlivoForwardRejectsSelfAndInvalidTargets(t *testing.T) {
	for _, target := range []string{"", "+918065589631", "<Number>123</Number>"} {
		w := httptest.NewRecorder()
		require.Error(t, writePlivoForward(w, "+918065589631", target, "https://calls.example/action", "https://calls.example/event"))
		require.Empty(t, w.Body.String())
	}
}
