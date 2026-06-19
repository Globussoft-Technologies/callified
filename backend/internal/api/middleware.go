package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// ctxKey is the context key for auth claims.
type ctxKey struct{}

// AuthClaims holds the fields extracted from a validated JWT.
type AuthClaims struct {
	Email string
	OrgID int64
	Role  string
}

// jwtClaims maps the Python-issued JWT payload.
// Python creates: {"sub": email, "org_id": org_id, "role": role, "exp": ...}
//
// Kind is empty for the regular long-lived auth JWT and "sse" for the
// short-lived ticket minted by /api/sse/ticket — the SSE-specific auth path
// rejects anything except kind="sse" so a leaked auth JWT can't be
// downgraded into a query-string ticket. (issue #80)
type jwtClaims struct {
	jwt.RegisteredClaims
	OrgID int64  `json:"org_id"`
	
	Role  string `json:"role"`
	Kind  string `json:"kind,omitempty"`
}

// requireAuth is middleware that validates the Bearer JWT and injects AuthClaims into context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr, err := bearerToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}

		claims := &jwtClaims{}
		_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ac := AuthClaims{
			Email: claims.Subject, // Python sets sub = email
			OrgID: claims.OrgID,
			Role:  claims.Role,
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, ac)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// getAuth retrieves AuthClaims from the request context.
func getAuth(r *http.Request) AuthClaims {
	v, _ := r.Context().Value(ctxKey{}).(AuthClaims)
	return v
}

// requireRole wraps requireAuth and additionally enforces that the
// authenticated user's role is one of the allowed values. Returns 403 with
// no body details on mismatch — we don't tell the caller which role they
// would need.
//
// Existing JWTs minted before role was added to the claims may have an empty
// Role field; in that case we fall back to a single DB lookup so a long-lived
// token doesn't accidentally bypass authorization. Subsequent re-logins
// embed the role and skip the lookup.
func (s *Server) requireRole(allowed ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			ac := getAuth(r)
			// Always resolve the role from DB instead of trusting the JWT
			// claim. Role changes by an Admin (e.g. promoting an Agent to
			// Admin or demoting to Viewer) must take effect immediately for
			// the affected user without forcing a re-login. Trusting the
			// claim cached the old role for the JWT's full TTL — users
			// reported "I changed my role but campaigns are still empty"
			// because their JWT still said Viewer.
			role := ac.Role
			if s.db != nil && ac.Email != "" {
				if u, err := s.db.GetUserByEmail(ac.Email); err == nil && u != nil {
					role = u.Role
				}
			}
			// Super-admins can access any role-gated endpoint.
			if s.isSuperAdmin(ac.Email) {
				next.ServeHTTP(w, r)
				return
			}
			for _, want := range allowed {
				if role == want {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "forbidden")
		})
	}
}

// bearerToken extracts the token string from "Authorization: Bearer <token>".
// Query-string fallback was removed — the long-lived auth JWT must never
// appear in URLs because reverse proxies, browser history, and Referer
// headers leak query strings. (issue #80) For SSE / <audio> tag callers
// that cannot set headers, see requireSSETicket and the blob-fetch pattern
// on the frontend.
func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", fmt.Errorf("no Authorization header")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", fmt.Errorf("expected Bearer token")
	}
	return parts[1], nil
}

// requireAPIKeyOrAuth accepts either an X-API-Key header (for partner /
// external integrations) or a Bearer JWT (for browser-driven dashboard calls).
// The two paths populate the same AuthClaims so downstream handlers don't need
// to branch.
//
// Spec-defined error responses:
//
//	401 {"error": "..."} — missing or unknown key / invalid JWT
//	403 {"error": "API key has been revoked"} — known key, is_active=0
//
// Key format: callers send the raw "ck_..." string in X-API-Key. We SHA-256
// it and look up by hash so the DB never holds the plaintext.
func (s *Server) requireAPIKeyOrAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if raw := strings.TrimSpace(r.Header.Get("X-API-Key")); raw != "" {
			sum := sha256.Sum256([]byte(raw))
			hashed := fmt.Sprintf("%x", sum)
			k, err := s.db.GetAPIKeyByHash(hashed)
			if err != nil {
				s.logger.Sugar().Errorw("api key lookup", "err", err)
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if k == nil {
				writeError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}
			if !k.IsActive {
				writeError(w, http.StatusForbidden, "API key has been revoked")
				return
			}
			// Fire-and-forget last_used_at bump; a write failure must not
			// reject an otherwise valid request.
			go func(id int64) { _ = s.db.TouchAPIKey(id) }(k.ID)

			ac := AuthClaims{
				Email: "apikey:" + k.KeyPrefix, // audit trail in logs
				OrgID: k.OrgID,
				Role:  "Admin", // external keys are org-scoped, treat as admin for read-only external endpoints
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, ac)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// No X-API-Key → fall back to JWT.
		s.requireAuth(next).ServeHTTP(w, r)
	}
}

// requireSSETicket is a middleware variant for SSE endpoints. It reads a
// short-lived ticket from the ?ticket= query (because EventSource cannot
// send custom headers) and accepts ONLY tokens with kind="sse" — the
// long-lived auth JWT is rejected here even if smuggled in. The ticket is
// minted via GET /api/sse/ticket which itself requires Bearer auth.
func (s *Server) requireSSETicket(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("ticket")
		if t == "" {
			writeError(w, http.StatusUnauthorized, "missing ticket")
			return
		}
		claims := &jwtClaims{}
		_, err := jwt.ParseWithClaims(t, claims, func(tok *jwt.Token) (any, error) {
			if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})
		if err != nil || claims.Kind != "sse" {
			writeError(w, http.StatusUnauthorized, "invalid or expired ticket")
			return
		}
		ac := AuthClaims{Email: claims.Subject, OrgID: claims.OrgID, Role: claims.Role}
		ctx := context.WithValue(r.Context(), ctxKey{}, ac)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
