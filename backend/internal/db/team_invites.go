package db

import (
	"database/sql"
	"errors"
	"time"
)

// TeamInvite mirrors a row from team_invites — used for the email-link invite
// flow that replaced the older "inviter sets the password directly" UX.
type TeamInvite struct {
	ID               int64
	Token            string
	OrgID            int64
	Email            string
	FullName         string
	Role             string
	InvitedByUserID  sql.NullInt64
	ExpiresAt        time.Time
	AcceptedAt       sql.NullTime
	CreatedAt        time.Time
}

// CreateTeamInvite inserts a fresh invite row. Caller generates the token
// (32 bytes of url-safe randomness, same convention as password_reset_tokens).
func (d *DB) CreateTeamInvite(orgID int64, email, fullName, role, token string, invitedBy int64, expiresAt time.Time) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO team_invites (org_id, email, full_name, role, token, invited_by_user_id, expires_at)
		 VALUES (?,?,?,?,?,?,?)`,
		orgID, email, fullName, role, token, invitedBy, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetValidTeamInvite returns the row only when it's unaccepted AND unexpired.
// Returns (nil, nil) on "no such valid invite" so handlers can produce a clean
// 410/400 without distinguishing expired vs accepted vs missing.
func (d *DB) GetValidTeamInvite(token string) (*TeamInvite, error) {
	row := d.pool.QueryRow(
		`SELECT id, token, org_id, email, full_name, role, invited_by_user_id,
		        expires_at, accepted_at, created_at
		 FROM team_invites
		 WHERE token=? AND accepted_at IS NULL AND expires_at > NOW()
		 LIMIT 1`, token)
	var t TeamInvite
	err := row.Scan(&t.ID, &t.Token, &t.OrgID, &t.Email, &t.FullName, &t.Role,
		&t.InvitedByUserID, &t.ExpiresAt, &t.AcceptedAt, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkTeamInviteAccepted flips accepted_at=NOW() so the token can never be
// replayed.
func (d *DB) MarkTeamInviteAccepted(id int64) error {
	_, err := d.pool.Exec(`UPDATE team_invites SET accepted_at=NOW() WHERE id=?`, id)
	return err
}

// HasPendingInviteForEmail reports whether an unaccepted, unexpired invite
// exists for (orgID, email). Used to refuse a duplicate invite before sending
// a second email.
func (d *DB) HasPendingInviteForEmail(orgID int64, email string) (bool, error) {
	var n int
	err := d.pool.QueryRow(
		`SELECT COUNT(*) FROM team_invites
		 WHERE org_id=? AND email=? AND accepted_at IS NULL AND expires_at > NOW()`,
		orgID, email).Scan(&n)
	return n > 0, err
}

// PendingInvite is the redacted shape returned to the team list — drops the
// token (only the invitee should ever see that) but exposes who's pending.
type PendingInvite struct {
	ID         int64     `json:"id"`
	Email      string    `json:"email"`
	FullName   string    `json:"full_name"`
	Role       string    `json:"role"`
	InvitedBy  string    `json:"invited_by"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListPendingInvites returns the unaccepted, unexpired invites for an org —
// used by the team-management UI to show who's been invited but hasn't joined
// yet. Joins users so the inviter's name surfaces without an extra round-trip.
func (d *DB) ListPendingInvites(orgID int64) ([]PendingInvite, error) {
	rows, err := d.pool.Query(
		`SELECT i.id, i.email, i.full_name, i.role,
		        COALESCE(u.full_name, u.email, ''),
		        i.expires_at, i.created_at
		 FROM team_invites i
		 LEFT JOIN users u ON u.id = i.invited_by_user_id
		 WHERE i.org_id=? AND i.accepted_at IS NULL AND i.expires_at > NOW()
		 ORDER BY i.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingInvite
	for rows.Next() {
		var p PendingInvite
		if err := rows.Scan(&p.ID, &p.Email, &p.FullName, &p.Role,
			&p.InvitedBy, &p.ExpiresAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetTeamInviteToken returns the raw token for a pending invite ID, scoped to
// org so an admin can never resolve another tenant's invite. Used by the
// "Copy invite link" action so the inviter can hand the link off out-of-band
// when SMTP is misconfigured or the invitee never received the email.
func (d *DB) GetTeamInviteToken(id, orgID int64) (string, error) {
	var token string
	err := d.pool.QueryRow(
		`SELECT token FROM team_invites
		 WHERE id=? AND org_id=? AND accepted_at IS NULL AND expires_at > NOW()
		 LIMIT 1`, id, orgID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return token, err
}

// DeleteTeamInvite removes a pending invite (Admin cancels before acceptance).
// Scoped to org so an admin in one org can't cancel another org's invite.
func (d *DB) DeleteTeamInvite(id, orgID int64) error {
	_, err := d.pool.Exec(
		`DELETE FROM team_invites WHERE id=? AND org_id=? AND accepted_at IS NULL`,
		id, orgID)
	return err
}

// GetOrgName returns the organization's display name — used in invite emails
// so the recipient sees "You've been invited to <Org Name>" instead of just
// the inviter's email.
func (d *DB) GetOrgName(orgID int64) (string, error) {
	var name string
	err := d.pool.QueryRow(`SELECT name FROM organizations WHERE id=?`, orgID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}
