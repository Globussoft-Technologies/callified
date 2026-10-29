package dial

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AgentTicketClaims authorizes one browser to join one org's manual call.
type AgentTicketClaims struct {
	jwt.RegisteredClaims
	OrgID  int64  `json:"org_id"`
	UserID int64  `json:"user_id"`
	Kind   string `json:"kind"`
}

func AgentTicket(secret, callSID string, orgID, userID int64) (string, error) {
	if secret == "" || callSID == "" || orgID <= 0 || userID < 0 {
		return "", fmt.Errorf("missing browser call identity")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, AgentTicketClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: callSID, ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Minute)),
		},
		OrgID: orgID, UserID: userID, Kind: "agent",
	}).SignedString([]byte(secret))
}

func VerifyAgentTicket(secret, token, callSID string) (*AgentTicketClaims, error) {
	claims := &AgentTicketClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || secret == "" || claims.Kind != "agent" || claims.Subject != callSID || claims.OrgID <= 0 || claims.UserID < 0 {
		return nil, fmt.Errorf("invalid browser call ticket")
	}
	return claims, nil
}
