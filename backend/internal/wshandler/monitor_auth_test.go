package wshandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMonitorRequiresTicketWhenJWTConfigured(t *testing.T) {
	h := &Handler{cfg: &config.Config{JWTSecret: "test-monitor-secret-at-least-32-bytes"}}
	req := httptest.NewRequest(http.MethodGet, "/ws/monitor/test-stream", nil)
	recorder := httptest.NewRecorder()

	h.ServeMonitor(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
