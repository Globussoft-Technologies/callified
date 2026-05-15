package api

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// ── SSO (JWT-based trusted-issuer flow) ──────────────────────────────────────
//
// External system mints a JWT signed with either:
//
//	• RS256 — issuer holds the private key, we hold the public key (recommended)
//	• HS256 — both sides share a secret (simpler, less safe across orgs)
//
// Required claims:
//
//	sub    — stable user identifier in the issuer's system (e.g. "emp-12345")
//	email  — used to find/create the user in Callified
//	exp    — unix-seconds expiry, validated by jwt lib
//	iat    — unix-seconds issued-at
//
// Optional but useful:
//
//	iss    — validated against cfg.SSOIssuer when set
//	aud    — validated against cfg.SSOAudience when set
//	role   — "Admin" or "Agent"; defaults to "Agent" on JIT-create
//	org_id — int; required when JIT-creating a brand new user (we won't
//	         guess which org a stranger belongs to)
//	name   — display name for JIT-create
//
// Flow:
//
//	1. Browser hits GET /api/auth/sso/jwt?token=<jwt>&redirect=/crm
//	2. Verify signature + claims
//	3. Find user by email; JIT-create if missing (org_id required)
//	4. Mint our own internal JWT (same shape login uses, role embedded)
//	5. 302 to <FrontendURL>/sso/return?token=<our-jwt>&next=<redirect>
//
// The frontend's /sso/return page reads ?token=, drops it into localStorage,
// fetches /api/auth/me, and navigates to ?next=. Same UX as the regular
// login flow once we hand off the token.

// ssoClaims is the JWT envelope we accept from the external issuer. Inline
// jwt.RegisteredClaims gives us automatic exp/nbf/iat validation.
type ssoClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	OrgID int64  `json:"org_id"`
}

// GET /api/auth/sso/jwt — public endpoint (no auth middleware)
func (s *Server) ssoJWT(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("token"))
	next := r.URL.Query().Get("redirect")
	if next == "" {
		next = "/crm"
	}

	if raw == "" {
		s.ssoFail(w, r, next, "missing_token", http.StatusBadRequest)
		return
	}

	// 1. Pick verification key based on which env var is set. Verify the
	//    token and parse the claims. Default jwt.ParseWithClaims validates
	//    exp/nbf/iat; iss/aud are validated below.
	keyfunc, alg, err := s.ssoKeyfunc()
	if err != nil {
		s.logger.Sugar().Warnw("ssoJWT: not configured", "err", err)
		writeError(w, http.StatusServiceUnavailable, "sso not configured")
		return
	}

	claims := &ssoClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, keyfunc,
		jwt.WithValidMethods([]string{alg}),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil || !tok.Valid {
		s.logger.Sugar().Infow("ssoJWT: invalid token", "err", err)
		s.ssoFail(w, r, next, "invalid_token", http.StatusUnauthorized)
		return
	}

	// 2. Validate iss / aud when configured. The lib has helpers for these
	//    via ParserOption, but we want to log which check failed for ops
	//    visibility, so do it manually.
	if want := s.cfg.SSOIssuer; want != "" && claims.Issuer != want {
		s.logger.Sugar().Infow("ssoJWT: issuer mismatch", "want", want, "got", claims.Issuer)
		s.ssoFail(w, r, next, "untrusted_issuer", http.StatusUnauthorized)
		return
	}
	if want := s.cfg.SSOAudience; want != "" && !audienceContains(claims.Audience, want) {
		s.logger.Sugar().Infow("ssoJWT: audience mismatch", "want", want, "got", claims.Audience)
		s.ssoFail(w, r, next, "wrong_audience", http.StatusUnauthorized)
		return
	}

	email := strings.TrimSpace(strings.ToLower(claims.Email))
	if email == "" {
		s.ssoFail(w, r, next, "missing_email", http.StatusBadRequest)
		return
	}

	// 3. Find existing user. If missing, JIT-create — but only when the
	//    issuer told us which org the user belongs to. We never guess: a
	//    stranger arriving without an org_id claim is rejected so a typo'd
	//    JWT can't drop someone into the first org we find.
	user, err := s.db.GetUserByEmail(email)
	if err != nil {
		s.logger.Sugar().Errorw("ssoJWT: GetUserByEmail", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Normalize the claim values once so JIT-create and existing-user-sync
	// see the same canonical role / org_id.
	claimRole := s.normalizeRole(claims.Role)
	claimOrg := s.remapOrgID(claims.OrgID)

	if user == nil {
		if claimOrg <= 0 {
			s.ssoFail(w, r, next, "org_required_for_jit_create", http.StatusForbidden)
			return
		}
		// Empty password hash → user can never log in via password; SSO is
		// the only path. CreateUser is happy with that today; if you want
		// to enforce at the DB layer, make password_hash NOT NULL with a
		// per-user random sentinel.
		uid, err := s.db.CreateUser(email, "", strings.TrimSpace(claims.Name), claimRole, claimOrg)
		if err != nil {
			s.logger.Sugar().Errorw("ssoJWT: CreateUser failed", "err", err, "email", email)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		s.logger.Sugar().Infow("ssoJWT: JIT-created user",
			"id", uid, "email", email, "role", claimRole, "org", claimOrg,
			"raw_role", claims.Role, "raw_org", claims.OrgID)
		user, err = s.db.GetUserByEmail(email)
		if err != nil || user == nil {
			s.logger.Sugar().Errorw("ssoJWT: post-create lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	} else {
		// Existing user: re-sync role / org_id from the JWT each login. The
		// partner system is the source of truth for "who is this user, what
		// org do they belong to, what permissions" — without this, fixing a
		// user via the DB has to happen for every new login because the next
		// SSO would have re-set the wrong values, and conversely a partner-
		// side role change wouldn't take effect until we manually touched
		// the row. Only updates when there's an actual delta so we don't
		// rewrite every row on every login.
		if claimRole != "" && (claimRole != user.Role || (claimOrg > 0 && claimOrg != user.OrgID)) {
			newRole := claimRole
			newOrg := user.OrgID
			if claimOrg > 0 {
				newOrg = claimOrg
			}
			if err := s.db.UpdateUserRoleAndOrg(user.ID, newRole, newOrg); err != nil {
				s.logger.Sugar().Warnw("ssoJWT: UpdateUserRoleAndOrg failed",
					"err", err, "user_id", user.ID, "email", email)
			} else {
				s.logger.Sugar().Infow("ssoJWT: synced role/org from JWT",
					"user_id", user.ID, "email", email,
					"from_role", user.Role, "to_role", newRole,
					"from_org", user.OrgID, "to_org", newOrg)
				user.Role = newRole
				user.OrgID = newOrg
			}
		}
	}

	// 4. Mint our own JWT (same shape the regular login mints — role
	//    embedded so requireRole middleware works without a DB roundtrip).
	out, err := s.mintToken(user.Email, user.OrgID, user.Role)
	if err != nil {
		s.logger.Sugar().Errorw("ssoJWT: mintToken failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// 5. Redirect the browser into the SPA with the token. /sso/return is a
	//    public route in App.jsx that stores the token, fetches /auth/me,
	//    then navigates to ?next=.
	dst := s.cfg.FrontendURL + "/sso/return?token=" + url.QueryEscape(out) +
		"&next=" + url.QueryEscape(next)
	http.Redirect(w, r, dst, http.StatusFound)
}

// ssoKeyfunc returns the jwt key-resolver paired with the algorithm we
// expect, based on which SSO env var is configured. Public-key beats
// shared-secret so an operator can roll out RS256 by adding the PEM without
// having to remove the legacy HS256 secret atomically.
func (s *Server) ssoKeyfunc() (jwt.Keyfunc, string, error) {
	if pemStr := strings.TrimSpace(s.cfg.SSOPublicKeyPEM); pemStr != "" {
		block, _ := pem.Decode([]byte(pemStr))
		if block == nil {
			return nil, "", errors.New("SSO_PUBLIC_KEY_PEM: not a PEM block")
		}
		var pub *rsa.PublicKey
		if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			rsaKey, ok := k.(*rsa.PublicKey)
			if !ok {
				return nil, "", errors.New("SSO public key: not RSA")
			}
			pub = rsaKey
		} else if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
			pub = k
		} else {
			return nil, "", fmt.Errorf("SSO public key parse: %w", err)
		}
		return func(*jwt.Token) (any, error) { return pub, nil }, "RS256", nil
	}
	if secret := s.cfg.SSOSharedSecret; secret != "" {
		return func(*jwt.Token) (any, error) { return []byte(secret), nil }, "HS256", nil
	}
	return nil, "", errors.New("set SSO_SHARED_SECRET or SSO_PUBLIC_KEY_PEM")
}

// ssoFail redirects back to the frontend with ?error=<code> so the SPA can
// render a friendly message rather than dumping a 401 JSON payload to the
// user. Keeps the failure UX uniform with the success path.
func (s *Server) ssoFail(w http.ResponseWriter, r *http.Request, next, code string, statusOnFallback int) {
	if s.cfg.FrontendURL == "" {
		writeError(w, statusOnFallback, code)
		return
	}
	dst := s.cfg.FrontendURL + "/sso/return?error=" + url.QueryEscape(code) +
		"&next=" + url.QueryEscape(next)
	_ = zap.String // silence unused import lint when logger sugaring not used here
	http.Redirect(w, r, dst, http.StatusFound)
}

// audienceContains returns true if want appears anywhere in aud. JWT's "aud"
// claim is technically a string-or-array; the jwt-go lib normalises both
// shapes into ClaimStrings so a simple slice scan covers both.
func audienceContains(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}

// normalizeRole accepts any case variant of a known role and returns the
// canonical capitalised form. Falls back to the configured default (or
// "Agent") when the input doesn't match a known role. Partner systems mint
// tokens with "admin" / "ADMIN" / "Owner" inconsistently and we don't want
// to bounce every new integration over case alone.
func (s *Server) normalizeRole(raw string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	switch r {
	case "admin":
		return "Admin"
	case "agent":
		return "Agent"
	case "viewer":
		return "Viewer"
	}
	fallback := strings.TrimSpace(s.cfg.SSODefaultRole)
	if fallback == "" {
		return "Agent"
	}
	// Run the fallback through the same switch so a misconfigured env
	// (e.g. SSO_DEFAULT_ROLE=admin) still produces a valid value.
	switch strings.ToLower(fallback) {
	case "admin":
		return "Admin"
	case "viewer":
		return "Viewer"
	}
	return "Agent"
}

// remapOrgID applies SSO_ORG_REMAP rules to the claim's org_id. Format of
// the env var is "from:to,from:to" — every match-as-source returns the
// target, otherwise the input is returned unchanged. Parsing is forgiving:
// malformed pairs are skipped silently so a typo can't take down SSO.
func (s *Server) remapOrgID(in int64) int64 {
	raw := strings.TrimSpace(s.cfg.SSOOrgRemap)
	if raw == "" || in <= 0 {
		return in
	}
	for _, pair := range strings.Split(raw, ",") {
		fromStr, toStr, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok {
			continue
		}
		var from, to int64
		if _, err := fmt.Sscanf(strings.TrimSpace(fromStr), "%d", &from); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(toStr), "%d", &to); err != nil {
			continue
		}
		if from == in {
			return to
		}
	}
	return in
}
