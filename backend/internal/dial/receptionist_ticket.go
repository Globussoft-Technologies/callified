package dial

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ReceptionistTicketClaims struct {
	jwt.RegisteredClaims
	OrgID  int64  `json:"org_id"`
	UserID int64  `json:"user_id"`
	Kind   string `json:"kind"`
}

func ReceptionistTicket(secret, stream string, orgID, userID int64) (string, error) {
	if secret == "" || stream == "" || orgID <= 0 || userID <= 0 {
		return "", fmt.Errorf("invalid simulator identity")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, ReceptionistTicketClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: stream, ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Minute))},
		OrgID:            orgID, UserID: userID, Kind: "receptionist_sim",
	}).SignedString([]byte(secret))
}

func VerifyReceptionistTicket(secret, token, stream string) (*ReceptionistTicketClaims, error) {
	claims := &ReceptionistTicketClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || secret == "" || claims.Kind != "receptionist_sim" || claims.Subject != stream || claims.OrgID <= 0 || claims.UserID <= 0 {
		return nil, fmt.Errorf("invalid simulator ticket")
	}
	return claims, nil
}
