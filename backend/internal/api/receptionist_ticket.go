package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/globussoft/callified-backend/internal/dial"
)

func (s *Server) receptionistTicket(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orgID <= 0 {
		writeError(w, 400, "invalid organization")
		return
	}
	if !s.authorizeOrg(w, r, orgID) {
		return
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		writeError(w, 503, "simulator unavailable")
		return
	}
	stream := "web_sim_inbound_" + hex.EncodeToString(nonce[:])
	ticket, err := dial.ReceptionistTicket(s.cfg.JWTSecret, stream, orgID, getAuth(r).UserID)
	if err != nil {
		writeError(w, 503, "simulator unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"stream_sid": stream, "ticket": ticket})
}
