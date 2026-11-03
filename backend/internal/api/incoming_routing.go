package api

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

func decodeRoutingBody(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "invalid request")
		return false
	}
	return true
}
func (s *Server) forwardingNumbers(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "provider_accounts.global") {
		return
	}
	orgID := getAuth(r).OrgID
	if r.Method == http.MethodGet {
		numbers, err := s.db.ListForwardingNumbers(orgID)
		if err != nil {
			writeError(w, 500, "could not load numbers")
			return
		}
		writeJSON(w, 200, numbers)
		return
	}
	var number db.ForwardingNumber
	if r.Method == http.MethodDelete {
		id, err := parseID(r, "id")
		if err != nil || id <= 0 {
			writeError(w, 400, "invalid number id")
			return
		}
		if err = s.db.DeleteForwardingNumber(orgID, id); err != nil {
			writeError(w, 409, "Could not delete number. Switch any routes using it to AI first.")
			return
		}
		w.WriteHeader(204)
		return
	}
	if !decodeRoutingBody(w, r, &number) {
		return
	}
	number.ID = 0
	if r.Method == http.MethodPut {
		id, err := parseID(r, "id")
		if err != nil || id <= 0 {
			writeError(w, 400, "invalid number id")
			return
		}
		number.ID = id
	}
	number.Name = strings.TrimSpace(number.Name)
	normalized := plivoNumber(number.Phone)
	if number.Name == "" || len(number.Name) > 100 || normalized == "" || normalized[0] == '0' {
		writeError(w, 400, "Enter a name and valid number with country code (or a 10-digit Indian number).")
		return
	}
	accounts, err := s.db.GetOrgExotelAccounts(orgID)
	if err != nil {
		writeError(w, 500, "could not validate number")
		return
	}
	for _, account := range accounts {
		if plivoNumber(account.CallerID) == normalized {
			writeError(w, 400, "Forwarding to your own provider number would create a call loop.")
			return
		}
	}
	number.Phone = "+" + normalized
	if err = s.db.SaveForwardingNumber(orgID, number); err != nil {
		writeError(w, 409, "Could not save number. Check that it exists and is not already saved.")
		return
	}
	writeJSON(w, 200, number)
}
func (s *Server) incomingRoute(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "provider_accounts.global") {
		return
	}
	orgID := getAuth(r).OrgID
	id, err := parseID(r, "id")
	if err != nil || id <= 0 {
		writeError(w, 400, "invalid account")
		return
	}
	account, err := s.db.GetExotelAccountByIDInOrg(id, orgID)
	if err != nil || account == nil || account.Provider != "plivo" {
		writeError(w, 404, "Plivo account not found")
		return
	}
	if r.Method == http.MethodGet {
		route, err := s.db.GetIncomingRoute(orgID, id)
		if err != nil {
			writeError(w, 500, "could not load routing")
			return
		}
		writeJSON(w, 200, route)
		return
	}
	var route db.IncomingRoute
	if !decodeRoutingBody(w, r, &route) {
		return
	}
	if route.Mode != "ai" && route.Mode != "phone" {
		writeError(w, 400, "Select AI or a forwarding number.")
		return
	}
	if route.Mode == "ai" {
		route.DestinationID = 0
	} else if route.DestinationID <= 0 {
		writeError(w, 400, "Select a saved forwarding number.")
		return
	}
	if err = s.db.SaveIncomingRoute(orgID, id, route); err != nil {
		writeError(w, 400, "Could not save routing. Select a number from your organization.")
		return
	}
	writeJSON(w, 200, route)
}

func writePlivoForward(w http.ResponseWriter, callerID, target string) error {
	from, to := plivoNumber(callerID), plivoNumber(target)
	if from == "" || to == "" || from == to {
		return fmt.Errorf("invalid forwarding destination")
	}
	response := struct {
		XMLName xml.Name `xml:"Response"`
		Dial    struct {
			CallerID  string `xml:"callerId,attr"`
			Timeout   int    `xml:"timeout,attr"`
			TimeLimit int    `xml:"timeLimit,attr"`
			Number    string `xml:"Number"`
		} `xml:"Dial"`
		Hangup string `xml:"Hangup"`
	}{}
	response.Dial.CallerID = "+" + from
	response.Dial.Number = "+" + to
	response.Dial.Timeout = 30
	response.Dial.TimeLimit = 3600
	body, err := xml.Marshal(response)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/xml")
	_, err = w.Write(append([]byte(xml.Header), body...))
	return err
}
