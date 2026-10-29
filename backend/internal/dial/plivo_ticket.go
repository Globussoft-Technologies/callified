package dial

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

func PlivoStreamTicket(secret, callID, requestID string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"kind": "plivo_stream", "sub": callID, "request_id": requestID, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte(secret))
}

func VerifyPlivoStreamTicket(secret, value string) (string, string, error) {
	token, err := jwt.Parse(value, func(t *jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return "", "", fmt.Errorf("invalid Plivo stream ticket")
	}
	c := token.Claims.(jwt.MapClaims)
	call, _ := c["sub"].(string)
	request, _ := c["request_id"].(string)
	if c["kind"] != "plivo_stream" || call == "" || request == "" {
		return "", "", fmt.Errorf("invalid Plivo stream scope")
	}
	return call, request, nil
}
