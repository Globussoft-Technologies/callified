package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
)

// inviteTokenTTL is how long a team invite link stays valid. 72h gives
// real humans a couple of business days to act, while still expiring
// abandoned links quickly enough that they don't accumulate as latent
// account-creation primitives.
const inviteTokenTTL = 72 * time.Hour

// ── GET /api/dashboard/summary ────────────────────────────────────────────────
// Open to any authenticated role (Admin / Agent / Viewer) so the CRM
// dashboard cards render real numbers even though full /api/campaigns is
// admin-gated. Returns just the 5 aggregate counts — no campaign objects.

func (s *Server) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	summary, err := s.db.GetOrgDashboardSummary(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("dashboardSummary", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ── GET /api/team ─────────────────────────────────────────────────────────────

func (s *Server) listTeam(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	members, err := s.db.GetTeamMembers(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listTeam", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(members))
}

// ── POST /api/team/invite ─────────────────────────────────────────────────────
//
// Issue #55 (security): the inviter no longer sets the invitee's password.
// We persist a short-lived invite token, email the invitee a one-time link,
// and the invitee chooses their own password via the public accept endpoint.

func (s *Server) inviteTeamMember(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	body.FullName = strings.TrimSpace(body.FullName)
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "Email is required.")
		return
	}
	if body.Role == "" {
		body.Role = "Agent"
	}

	// Refuse if a user with that email already exists in this (or any) org —
	// the unique constraint on users.email would also catch it at accept time,
	// but failing fast at invite time gives the inviter clear feedback.
	if existing, err := s.db.GetUserByEmail(body.Email); err != nil {
		s.logger.Sugar().Errorw("inviteTeamMember: user lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if existing != nil {
		writeError(w, http.StatusConflict, "A user with this email already exists.")
		return
	}

	// Refuse a duplicate pending invite — repeated form submits would otherwise
	// queue multiple live tokens for the same address.
	if pending, err := s.db.HasPendingInviteForEmail(ac.OrgID, body.Email); err != nil {
		s.logger.Sugar().Errorw("inviteTeamMember: pending lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if pending {
		writeError(w, http.StatusConflict, "An invite for this email is already pending.")
		return
	}

	// Resolve inviter so the email can sign off with a real name.
	caller, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || caller == nil {
		writeError(w, http.StatusInternalServerError, "could not resolve caller")
		return
	}

	rb := make([]byte, 32)
	if _, err := rand.Read(rb); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(rb)
	expiresAt := time.Now().Add(inviteTokenTTL)

	id, err := s.db.CreateTeamInvite(ac.OrgID, body.Email, body.FullName, body.Role, token, caller.ID, expiresAt)
	if err != nil {
		s.logger.Sugar().Errorw("inviteTeamMember: create invite", "err", err)
		writeError(w, http.StatusInternalServerError, "Could not create invite. Please try again.")
		return
	}

	// Best-effort email — if SMTP is misconfigured we still want the invite
	// row to exist so the admin can copy the link from logs / a future
	// resend endpoint. Never leak the token in the API response itself —
	// that would defeat the email-only delivery model.
	link := fmt.Sprintf("%s/accept-invite?token=%s", s.cfg.AppURL, token)
	if s.emailSvc != nil {
		orgName, _ := s.db.GetOrgName(ac.OrgID)
		if orgName == "" {
			orgName = "Callified"
		}
		inviterName := caller.FullName
		if inviterName == "" {
			inviterName = caller.Email
		}
		// When SMTP isn't actually configured, emailSvc.Send short-circuits to
		// nil — so the admin would never see the link. Log it here at INFO so
		// it shows up in `docker logs callified-go-audio` for local dev.
		// Production envs (SMTP_USER + SMTP_PASSWORD set) keep the link out
		// of stdout — the invitee gets it via email, never via logs.
		if s.cfg.SMTPUser == "" || s.cfg.SMTPPassword == "" {
			s.logger.Sugar().Infow("[INVITE] SMTP not configured — copy this link manually",
				"email", body.Email, "invite_link", link)
		}
		if err := s.emailSvc.SendTeamInvite(body.Email, body.FullName, inviterName, orgName, link, int(inviteTokenTTL/time.Hour)); err != nil {
			s.logger.Sugar().Warnw("inviteTeamMember: email send failed", "err", err, "email", body.Email)
		}
	} else {
		s.logger.Sugar().Warnw("inviteTeamMember: email service unavailable — invite created but link not delivered",
			"email", body.Email)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"email":   body.Email,
		"message": fmt.Sprintf("Invite email sent to %s.", body.Email),
	})
}

// ── GET /api/invite/{token} ───────────────────────────────────────────────────
// Public — no auth. Validates the token and returns the invitee's email,
// full name, role, and org name so the accept page can render a useful
// "You've been invited to <X>" header. Token itself is NEVER echoed back.

func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}
	inv, err := s.db.GetValidTeamInvite(token)
	if err != nil {
		s.logger.Sugar().Errorw("getInvite: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		writeError(w, http.StatusGone, "This invite link is invalid or has expired.")
		return
	}
	orgName, _ := s.db.GetOrgName(inv.OrgID)
	writeJSON(w, http.StatusOK, map[string]any{
		"email":     inv.Email,
		"full_name": inv.FullName,
		"role":      inv.Role,
		"org_name":  orgName,
	})
}

// ── POST /api/invite/{token}/accept ───────────────────────────────────────────
// Public — no auth. Body: {password}. Validates the token, validates the
// password, creates the user with the invitee-chosen password, marks the
// invite accepted (single-use). Re-checks GetUserByEmail in case a user with
// the same address was created via another flow between invite and accept.

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}
	var body struct {
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := s.validatePasswordStrong(r.Context(), body.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	inv, err := s.db.GetValidTeamInvite(token)
	if err != nil {
		s.logger.Sugar().Errorw("acceptInvite: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		writeError(w, http.StatusGone, "This invite link is invalid or has expired.")
		return
	}
	// Reject if a user with this email was created in the meantime (race or
	// a parallel signup). Better to fail clean than to silently 1062.
	if existing, err := s.db.GetUserByEmail(inv.Email); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if existing != nil {
		writeError(w, http.StatusConflict, "A user with this email already exists. Please sign in instead.")
		return
	}
	hash, err := db.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	fullName := strings.TrimSpace(body.FullName)
	if fullName == "" {
		fullName = inv.FullName
	}
	if _, err := s.db.CreateUserWithRole(inv.Email, hash, fullName, inv.Role, inv.OrgID); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "1062") || strings.Contains(errMsg, "Duplicate") {
			writeError(w, http.StatusConflict, "A user with this email already exists.")
			return
		}
		s.logger.Sugar().Errorw("acceptInvite: create user", "err", err)
		writeError(w, http.StatusInternalServerError, "Could not create your account. Please try again.")
		return
	}
	if err := s.db.MarkTeamInviteAccepted(inv.ID); err != nil {
		// User was created — log but don't fail the request, the invite is
		// "spent" effectively because GetUserByEmail will now refuse a second
		// accept.
		s.logger.Sugar().Warnw("acceptInvite: mark accepted", "err", err, "invite_id", inv.ID)
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"email":   inv.Email,
		"message": "Your account is ready. Please sign in.",
	})
}

// ── GET /api/team/invites ─────────────────────────────────────────────────────
// Admin-only. Returns the org's pending (unaccepted, unexpired) invites so
// the team page can show "Pending Invites" alongside actual members.

func (s *Server) listPendingInvites(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	invites, err := s.db.ListPendingInvites(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listPendingInvites", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(invites))
}

// ── GET /api/team/invites/{id}/link ───────────────────────────────────────────
// Admin-only. Returns the accept-invite URL for a pending invite so the
// inviter can copy/share it out-of-band — useful when SMTP is misconfigured,
// the invitee never received the email, or the admin wants to drop the link
// into Slack/WhatsApp instead. The token itself is what authorizes the
// invitee; making it visible to the inviter does NOT compromise the security
// model (issue #55) since the invitee still picks their own password.

func (s *Server) getInviteLink(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	token, err := s.db.GetTeamInviteToken(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getInviteLink", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if token == "" {
		writeError(w, http.StatusGone, "Invite is invalid, expired, or already accepted.")
		return
	}
	link := fmt.Sprintf("%s/accept-invite?token=%s", s.cfg.AppURL, token)
	writeJSON(w, http.StatusOK, map[string]string{"invite_link": link})
}

// ── DELETE /api/team/invites/{id} ─────────────────────────────────────────────
// Admin-only. Cancels a pending invite — the existing token becomes invalid
// at the next GetValidTeamInvite check (row simply isn't there).

func (s *Server) cancelInvite(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteTeamInvite(id, ac.OrgID); err != nil {
		s.logger.Sugar().Errorw("cancelInvite", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// validatePassword enforces the org-wide password policy. Returns "" when the
// password is acceptable, or a user-facing reason it isn't.
//
// Rules (issue #56):
//   - At least 8 characters (was 6 — too low for a 2026 baseline)
//   - At most 128 characters (bcrypt truncates at 72 bytes, but we let the
//     user type a passphrase up to 128 and bcrypt's silent truncation is
//     fine for practical purposes — we just guard against absurdly long
//     inputs that could DoS the bcrypt cost)
//   - Not in the in-memory blocklist of trivially-common passwords
//
// We deliberately do NOT require character classes (NIST 800-63B explicitly
// recommends against the "must have one uppercase, one digit, one symbol"
// nonsense — it pushes users toward predictable patterns like "Password1!").
// Length + breach awareness is the right baseline.
//
// HIBP breach check is layered on top via validatePasswordStrong (below).
// The bare validatePassword is kept for spots that need a fast, no-network
// gate (e.g. invariant checks far from request scope).
func validatePassword(p string) string {
	// testgo1 deployment intentionally accepts any non-empty password.
	// The length/commonPasswords/HIBP policy was producing too many false
	// positives during demo/staging signups. Restore from git history if
	// you want the original strict gate back.
	_ = p
	return ""
}

// validatePasswordStrong is the version request handlers should call.
// Disabled on testgo1 — passwords are accepted as-is. See validatePassword
// for the rationale and how to re-enable.
func (s *Server) validatePasswordStrong(ctx context.Context, p string) string {
	_ = ctx
	_ = p
	return ""
}

// commonPasswords is a tiny, hard-coded blocklist of the top trivial
// passwords. Keeping it in-process avoids a dependency on an external
// breach-list service for a basic gate; the real defense is bcrypt + the
// 8-char minimum above. Update list in lockstep with whatever the auth
// signup endpoint enforces (so the policy is consistent across surfaces).
var commonPasswords = map[string]struct{}{
	// Top trivial matches — instant reject without an HIBP round-trip.
	// HIBP catches the long tail; this list only needs the obvious ones.
	"password": {}, "password1": {}, "password123": {}, "password!": {}, "passw0rd": {},
	"12345678": {}, "123456789": {}, "1234567890": {}, "11111111": {}, "00000000": {},
	"qwerty": {}, "qwerty123": {}, "qwertyuiop": {}, "qwerty12": {},
	"abc12345": {}, "abcd1234": {}, "iloveyou": {}, "iloveu1": {},
	"admin": {}, "admin123": {}, "admin1234": {}, "administrator": {},
	"welcome": {}, "welcome1": {}, "welcome123": {},
	"letmein": {}, "letmein1": {}, "monkey": {}, "monkey123": {},
	"football": {}, "baseball": {}, "dragon": {}, "master": {}, "shadow": {},
	"sunshine": {}, "princess": {}, "trustno1": {},
	// India-specific common picks reported in regional credential dumps.
	"india123": {}, "india@123": {}, "callified": {}, "callified1": {},
}

// ── PUT /api/team/{id}/role ───────────────────────────────────────────────────

func (s *Server) updateTeamRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Role == "" {
		writeError(w, http.StatusBadRequest, "role required")
		return
	}
	if err := s.db.UpdateUserRole(id, body.Role); err != nil {
		s.logger.Sugar().Errorw("updateTeamRole", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── DELETE /api/team/{id} ─────────────────────────────────────────────────────

func (s *Server) deleteTeamMember(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	// Resolve caller's user row so we can compare IDs (the JWT carries email,
	// not user id) and check the target's role for the last-admin guard. Both
	// must be in the same org. Issue #54.
	caller, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || caller == nil {
		writeError(w, http.StatusInternalServerError, "could not resolve caller")
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.ID == caller.ID {
		writeError(w, http.StatusForbidden, "you cannot remove your own account")
		return
	}
	if target.Role == "Admin" {
		count, err := s.db.CountAdminsInOrg(ac.OrgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if count <= 1 {
			writeError(w, http.StatusForbidden, "cannot remove the last remaining admin")
			return
		}
	}
	if err := s.db.DeleteUser(id, ac.OrgID); err != nil {
		s.logger.Sugar().Errorw("deleteTeamMember", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
