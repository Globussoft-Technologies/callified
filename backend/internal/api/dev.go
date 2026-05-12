package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// ── Developer dashboard — password-less impersonation ────────────────────────
//
// Three endpoints gated by an env-var allowlist (DEVELOPER_EMAILS):
//
//   GET  /api/dev/users                  — paginated user list (allowlist-gated)
//   POST /api/dev/impersonate            — mint a one-shot key for a target user
//   POST /api/dev/impersonate/exchange   — swap key → JWT (public, but key is
//                                          unguessable + 60s TTL + single-use)
//
// The impersonation JWT carries a `dev_actor` claim (the developer's email) so
// every API call made during the impersonated session is traceable back to the
// human who initiated it. See middleware.go for AuthClaims.DevActor.

const (
	devImpersonateKeyTTL = 60 * time.Second
	devImpersonateNS     = "dev:impersonate:"
)

// requireDeveloper wraps requireAuth and rejects callers whose email is not
// in cfg.DeveloperEmails. Returns 404 (not 403) so the endpoint surface is
// invisible to probes — a non-developer can't tell whether /api/dev/* exists.
func (s *Server) requireDeveloper(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		ac := getAuth(r)
		if !s.cfg.IsDeveloper(ac.Email) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /api/dev/users?page=1&limit=25&search=&status=
func (s *Server) handleDevListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}
	search := q.Get("search")
	status := q.Get("status")

	offset := (page - 1) * limit
	rows, total, err := s.db.ListAllUsers(search, status, limit, offset)
	if err != nil {
		s.logger.Sugar().Errorw("dev: ListAllUsers", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	totalPages := (total + limit - 1) / limit
	writeJSON(w, http.StatusOK, map[string]any{
		"page":        page,
		"limit":       limit,
		"count":       len(rows),
		"total":       total,
		"total_pages": totalPages,
		"users":       rows,
	})
}

// POST /api/dev/impersonate  { "user_id": 123 }
//
// Mints a 30-day JWT for the target user, stashes it in Redis under a random
// 20-char key with a 60-second TTL, returns the key only. The caller exchanges
// the key for the JWT via /api/dev/impersonate/exchange.
func (s *Server) handleDevImpersonate(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)

	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID <= 0 {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}

	target, err := s.db.GetUserByID(req.UserID)
	if err != nil {
		s.logger.Sugar().Errorw("dev: GetUserByID", "err", err, "user_id", req.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	token, err := s.mintImpersonationToken(target.Email, target.OrgID, target.Role, ac.Email)
	if err != nil {
		s.logger.Sugar().Errorw("dev: mintImpersonationToken", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	key, err := randomKey()
	if err != nil {
		s.logger.Sugar().Errorw("dev: randomKey", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.store.SetRaw(r.Context(), devImpersonateNS+key, token, devImpersonateKeyTTL)

	s.logger.Info("dev_impersonate",
		zap.String("actor", ac.Email),
		zap.String("target", target.Email),
		zap.Int64("target_id", target.ID),
		zap.Int64("target_org_id", target.OrgID),
		zap.String("target_role", target.Role),
		zap.String("remote_addr", r.RemoteAddr),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"expires_in": int(devImpersonateKeyTTL.Seconds()),
	})
}

// POST /api/dev/impersonate/exchange  { "key": "..." }
//
// Public endpoint — the caller is the just-opened impersonation tab and has
// no JWT yet. Security comes from the key: 120 bits of entropy, 60s TTL,
// single-use (the read deletes the key). Replay → 404.
func (s *Server) handleDevExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}
	k := devImpersonateNS + req.Key
	token, ok := s.store.GetRaw(r.Context(), k)
	if !ok || token == "" {
		http.NotFound(w, r)
		return
	}
	// Single-use — invalidate before responding so a network retry can't
	// re-deliver the same JWT.
	s.store.DeleteRaw(r.Context(), k)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"token_type": "bearer",
	})
}

// mintImpersonationToken signs a regular 30-day auth JWT for `email` with a
// `dev_actor` claim recording the developer who initiated the impersonation.
// Shape is otherwise identical to s.mintToken so the auth middleware accepts
// it without any special-case path.
func (s *Server) mintImpersonationToken(email string, orgID int64, role, devActor string) (string, error) {
	claims := &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		},
		OrgID:    orgID,
		Role:     role,
		DevActor: devActor,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
}

// randomKey returns a URL-safe 20-character random string (15 bytes of entropy,
// base64.RawURL-encoded). Used as the opaque handoff key for impersonation.
func randomKey() (string, error) {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
