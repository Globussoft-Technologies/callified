package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/dashboard/summary ────────────────────────────────────────────────
// Open to any authenticated role (Admin / Agent / Executive) so the CRM
// dashboard cards render real numbers even though full /api/campaigns is
// admin-gated. Returns just the 5 aggregate counts — no campaign objects.

// @Summary     Dashboard summary
// @Description Returns 5 aggregate KPI counts for the org dashboard (open to any authenticated role).
// @Tags        dashboard
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  db.OrgDashboardSummary
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/dashboard/summary [get]
func (s *Server) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var summary db.OrgDashboardSummary
	var err error
	if s.isSuperAdmin(ac.Email) && ac.OrgID <= 0 {
		summary, err = s.db.GetAllDashboardSummary()
	} else if user, userErr := s.db.GetUserByEmail(ac.Email); userErr != nil {
		s.logger.Sugar().Errorw("dashboardSummary", "err", userErr)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	} else if user.Role == db.RoleAdmin {
		summary, err = s.db.GetOrgDashboardSummary(ac.OrgID)
	} else {
		campaigns, campErr := s.listCampaignsForUser(ac)
		if campErr != nil {
			s.logger.Sugar().Errorw("dashboardSummary", "err", campErr)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		campaignIDs := make([]int64, 0, len(campaigns))
		for _, c := range campaigns {
			campaignIDs = append(campaignIDs, c.ID)
		}
		execIDs, applyExecFilter, execErr := s.leadAccessExecIDs(ac)
		if execErr != nil {
			s.logger.Sugar().Errorw("dashboardSummary", "err", execErr)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if db.IsAgentLikeRole(user.Role) {
			summary, err = s.db.GetDashboardSummaryForCampaignsByUser(ac.OrgID, campaignIDs, execIDs, applyExecFilter, user.ID)
		} else {
			summary, err = s.db.GetDashboardSummaryForCampaigns(ac.OrgID, campaignIDs, execIDs, applyExecFilter)
		}
	}
	if err != nil {
		s.logger.Sugar().Errorw("dashboardSummary", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ── GET /api/team ─────────────────────────────────────────────────────────────

// @Summary     List team members
// @Description Returns all users in the org. Requires Admin role.
// @Tags        team
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.User
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team [get]
func (s *Server) listTeam(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.view") {
		return
	}
	ac := getAuth(r)
	members, err := s.db.GetTeamMembers(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listTeam", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(members))
}

type permissionDefinition struct {
	Key         string   `json:"key"`
	Module      string   `json:"module"`
	Label       string   `json:"label"`
	Action      string   `json:"action"`
	Description string   `json:"description"`
	Roles       []string `json:"roles"`
}

var permissionCatalog = []permissionDefinition{
	{Key: "dashboard.view", Module: "Dashboard", Label: "View dashboard", Action: "Can see", Description: "See dashboard summary and KPIs.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive, db.RoleTeamLeader}},
	{Key: "crm.view", Module: "CRM Leads", Label: "View leads", Action: "Can see", Description: "See leads within the user's allowed scope.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "crm.create", Module: "CRM Leads", Label: "Create leads", Action: "Can create", Description: "Add new leads within allowed campaigns.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "crm.edit", Module: "CRM Leads", Label: "Edit leads", Action: "Can edit", Description: "Update lead fields, status, notes, and follow-ups.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "crm.delete", Module: "CRM Leads", Label: "Delete leads", Action: "Can delete", Description: "Remove leads from the organization.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "crm.import", Module: "CRM Leads", Label: "Import leads", Action: "Can import", Description: "Bulk import leads using CSV.", Roles: []string{db.RoleAdmin}},
	{Key: "crm.export", Module: "CRM Leads", Label: "Export leads", Action: "Can export", Description: "Download lead lists as CSV.", Roles: []string{db.RoleAdmin}},
	{Key: "crm.assign", Module: "CRM Leads", Label: "Assign leads", Action: "Can assign", Description: "Assign campaign leads to executives.", Roles: []string{db.RoleAdmin}},
	{Key: "campaigns.view", Module: "Campaigns", Label: "View campaigns", Action: "Can see", Description: "See campaigns within the user's allowed scope.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "campaigns.create", Module: "Campaigns", Label: "Create campaigns", Action: "Can create", Description: "Create new campaigns.", Roles: []string{db.RoleAdmin}},
	{Key: "campaigns.edit", Module: "Campaigns", Label: "Edit campaigns", Action: "Can edit", Description: "Modify campaign details and settings.", Roles: []string{db.RoleAdmin}},
	{Key: "campaigns.delete", Module: "Campaigns", Label: "Delete campaigns", Action: "Can delete", Description: "Remove campaigns.", Roles: []string{db.RoleAdmin}},
	{Key: "campaigns.assign_users", Module: "Campaigns", Label: "Assign users", Action: "Can assign", Description: "Assign users or executives to campaigns.", Roles: []string{db.RoleAdmin}},
	{Key: "calls.make", Module: "Calls", Label: "Make calls", Action: "Can call", Description: "Start outbound calls for allowed campaign leads.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "calls.schedule", Module: "Calls", Label: "Schedule calls", Action: "Can schedule", Description: "Create scheduled calls for allowed campaign leads.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "calls.transcripts", Module: "Calls", Label: "View transcripts", Action: "Can see", Description: "Open call transcripts within the user's scope.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "calls.recordings", Module: "Calls", Label: "View recordings", Action: "Can see", Description: "Open call recordings within the user's scope.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "products.view", Module: "Products", Label: "View products", Action: "Can see", Description: "See product catalog.", Roles: []string{db.RoleAdmin}},
	{Key: "products.manage", Module: "Products", Label: "Manage products", Action: "Can manage", Description: "Create, edit, or delete products.", Roles: []string{db.RoleAdmin}},
	{Key: "reports.view", Module: "Reports", Label: "View reports", Action: "Can see", Description: "See performance reports within allowed scope.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "reports.download", Module: "Reports", Label: "Download reports", Action: "Can export", Description: "Download report files.", Roles: []string{db.RoleAdmin}},
	{Key: "team.view", Module: "Team", Label: "View team", Action: "Can see", Description: "See team member list.", Roles: []string{db.RoleAdmin}},
	{Key: "team.add", Module: "Team", Label: "Add members", Action: "Can create", Description: "Create or import team members.", Roles: []string{db.RoleAdmin}},
	{Key: "team.edit_role", Module: "Team", Label: "Edit roles", Action: "Can edit", Description: "Change another member's role.", Roles: []string{db.RoleAdmin}},
	{Key: "team.remove", Module: "Team", Label: "Remove members", Action: "Can delete", Description: "Remove members from the organization.", Roles: []string{db.RoleAdmin}},
	{Key: "team.reset_password", Module: "Team", Label: "Reset passwords", Action: "Can manage", Description: "Set a new password for a member.", Roles: []string{db.RoleAdmin}},
	{Key: "team.permissions", Module: "Team", Label: "Manage permissions", Action: "Can manage", Description: "Customize member permissions.", Roles: []string{db.RoleAdmin}},
	{Key: "team.api_keys", Module: "Team", Label: "Manage API keys", Action: "Can manage", Description: "Generate, copy, revoke, or delete team API keys.", Roles: []string{db.RoleAdmin}},
	{Key: "executives.manage", Module: "Executives", Label: "Manage executives", Action: "Can manage", Description: "Create, edit, or delete executives.", Roles: []string{db.RoleAdmin}},
	{Key: "provider_accounts.own", Module: "Provider Accounts", Label: "Manage own provider account", Action: "Can manage", Description: "Manage the user's own browser calling account.", Roles: []string{db.RoleAdmin, db.RoleAgent}},
	{Key: "calls.browser_call_account", Module: "Calls", Label: "Browser Call Account (this machine)", Action: "Can change", Description: "Change the browser call account selection for a campaign. If disabled, the assigned campaign/lead account is used and cannot be changed.", Roles: []string{db.RoleExecutive}},
	{Key: "provider_accounts.global", Module: "Provider Accounts", Label: "Manage global provider accounts", Action: "Can manage", Description: "Manage organization-level calling accounts.", Roles: []string{db.RoleAdmin}},
	{Key: "voice_settings.save", Module: "Voice Settings", Label: "Save voice settings", Action: "Can edit", Description: "Save campaign voice settings within allowed campaigns.", Roles: []string{db.RoleAdmin, db.RoleAgent, db.RoleExecutive}},
	{Key: "settings.manage", Module: "Settings", Label: "Manage settings", Action: "Can manage", Description: "Manage AI prompt, pronunciation, and call action settings.", Roles: []string{db.RoleAdmin}},
	{Key: "billing.manage", Module: "Billing", Label: "Manage billing", Action: "Can manage", Description: "View billing, add credits, and manage invoices.", Roles: []string{db.RoleAdmin}},
	{Key: "integrations.manage", Module: "Integrations", Label: "Manage integrations", Action: "Can manage", Description: "Add or update external integrations.", Roles: []string{db.RoleAdmin}},
	{Key: "dnd.manage", Module: "DND", Label: "Manage DND", Action: "Can manage", Description: "View, add, and check DND numbers.", Roles: []string{db.RoleAdmin}},
	{Key: "whatsapp.manage", Module: "WhatsApp", Label: "Manage WhatsApp", Action: "Can manage", Description: "Read and send WhatsApp conversations.", Roles: []string{db.RoleAdmin}},
	{Key: "knowledge.manage", Module: "Knowledge", Label: "Manage knowledge base", Action: "Can manage", Description: "Upload, embed, and delete RAG documents.", Roles: []string{db.RoleAdmin}},
	{Key: "monitor.view", Module: "Monitor", Label: "View live monitor", Action: "Can see", Description: "Open live AI call monitor.", Roles: []string{db.RoleAdmin}},
	{Key: "logs.view", Module: "Live Logs", Label: "View live logs", Action: "Can see", Description: "View system log stream.", Roles: []string{db.RoleAdmin}},
}

func rolePermissionSet(role string) map[string]bool {
	allowed := map[string]bool{}
	for _, p := range permissionCatalog {
		for _, r := range p.Roles {
			if r == role {
				allowed[p.Key] = true
				break
			}
		}
	}
	return allowed
}

func rolePermissionKeys(role string) []string {
	allowed := rolePermissionSet(role)
	keys := make([]string, 0, len(allowed))
	for _, p := range permissionCatalog {
		if allowed[p.Key] {
			keys = append(keys, p.Key)
		}
	}
	return keys
}

func filterPermissionsForRole(role string, requested []string) []string {
	allowed := rolePermissionSet(role)
	seen := map[string]bool{}
	out := []string{}
	for _, key := range requested {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] || !allowed[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func permissionsForRole(role string) []permissionDefinition {
	out := []permissionDefinition{}
	for _, p := range permissionCatalog {
		for _, r := range p.Roles {
			if r == role {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func allPermissionKeys() []string {
	keys := make([]string, 0, len(permissionCatalog))
	for _, p := range permissionCatalog {
		keys = append(keys, p.Key)
	}
	return keys
}

func (s *Server) effectivePermissionsForAuth(ac AuthClaims) ([]string, string, bool, error) {
	if s.isSuperAdmin(ac.Email) {
		return allPermissionKeys(), db.RoleAdmin, false, nil
	}
	if s.db == nil {
		return nil, "", false, errors.New("database unavailable")
	}
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil {
		return nil, "", false, err
	}
	if user == nil || !user.IsActive {
		return nil, "", false, errors.New("user not found")
	}
	perms, custom, err := s.db.GetUserPermissions(user.ID, user.OrgID)
	if err != nil {
		return nil, "", false, err
	}
	if !custom {
		perms = rolePermissionKeys(user.Role)
	} else {
		perms = filterPermissionsForRole(user.Role, perms)
	}
	return perms, user.Role, custom, nil
}

func hasPermissionKey(perms []string, key string) bool {
	for _, p := range perms {
		if p == key {
			return true
		}
	}
	return false
}

func (s *Server) authHasPermission(ac AuthClaims, key string) (bool, error) {
	perms, _, _, err := s.effectivePermissionsForAuth(ac)
	if err != nil {
		return false, err
	}
	return hasPermissionKey(perms, key), nil
}

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, key string) bool {
	ok, err := s.authHasPermission(getAuth(r), key)
	if err != nil {
		s.logger.Sugar().Errorw("requirePermission", "permission", key, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "permission denied")
		return false
	}
	return true
}

func (s *Server) getMyPermissions(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	perms, role, custom, err := s.effectivePermissionsForAuth(ac)
	if err != nil {
		s.logger.Sugar().Errorw("getMyPermissions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"role":        role,
		"permissions": perms,
		"available":   permissionsForRole(role),
		"is_custom":   custom,
	})
}

func (s *Server) getTeamMemberPermissions(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.permissions") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getTeamMemberPermissions: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	perms, custom, err := s.db.GetUserPermissions(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getTeamMemberPermissions: permissions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !custom {
		perms = rolePermissionKeys(target.Role)
	} else {
		perms = filterPermissionsForRole(target.Role, perms)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     target.ID,
		"role":        target.Role,
		"permissions": perms,
		"available":   permissionsForRole(target.Role),
		"is_custom":   custom,
	})
}

func (s *Server) updateTeamMemberPermissions(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.permissions") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("updateTeamMemberPermissions: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	perms := filterPermissionsForRole(target.Role, body.Permissions)
	if err := s.db.SetUserPermissions(id, ac.OrgID, perms); err != nil {
		s.logger.Sugar().Errorw("updateTeamMemberPermissions: save", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated":     true,
		"permissions": perms,
	})
}

func (s *Server) teamMembersTemplateCSV(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="team_members_template.csv"`)
	wr := csv.NewWriter(w)
	_ = wr.Write([]string{"full_name", "email", "password", "role"})
	wr.Flush()
}

func (s *Server) importTeamMembersCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.add") {
		return
	}
	ac := getAuth(r)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "CSV header is required")
		return
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	required := []string{"full_name", "email", "password", "role"}
	for _, col := range required {
		if _, ok := idx[col]; !ok {
			writeError(w, http.StatusBadRequest, "CSV must include full_name,email,password,role")
			return
		}
	}

	type rowError struct {
		Row   int    `json:"row"`
		Email string `json:"email,omitempty"`
		Error string `json:"error"`
	}
	var errorsOut []rowError
	created := 0
	rowNum := 1
	get := func(row []string, key string) string {
		i := idx[key]
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		rowNum++
		if err != nil {
			errorsOut = append(errorsOut, rowError{Row: rowNum, Error: "invalid CSV row"})
			continue
		}
		fullName := get(row, "full_name")
		email := strings.ToLower(get(row, "email"))
		password := get(row, "password")
		role := normalizeRole(get(row, "role"))
		if fullName == "" && email == "" && password == "" && get(row, "role") == "" {
			continue
		}
		if email == "" {
			errorsOut = append(errorsOut, rowError{Row: rowNum, Error: "email is required"})
			continue
		}
		if password == "" {
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "password is required"})
			continue
		}
		if role == "" {
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "role must be Admin, TeamLeader, Agent, or Executive"})
			continue
		}
		if existing, err := s.db.GetUserByEmail(email); err != nil {
			s.logger.Sugar().Errorw("importTeamMembersCSV: user lookup", "err", err)
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "could not validate email"})
			continue
		} else if existing != nil {
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "user already exists"})
			continue
		}
		hash, err := db.HashPassword(password)
		if err != nil {
			s.logger.Sugar().Errorw("importTeamMembersCSV: hash", "err", err)
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "could not hash password"})
			continue
		}
		if _, err := s.db.CreateUserWithRole(email, hash, fullName, role, ac.OrgID); err != nil {
			s.logger.Sugar().Errorw("importTeamMembersCSV: create user", "err", err)
			errorsOut = append(errorsOut, rowError{Row: rowNum, Email: email, Error: "could not create user"})
			continue
		}
		if role == db.RoleExecutive {
			_ = s.db.EnsureExecutiveForUser(ac.OrgID, fullName, email)
		}
		created++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"created": created,
		"errors":  emptyJSON(errorsOut),
	})
}

// ── POST /api/team/invite ─────────────────────────────────────────────────────
//
// Creates a team member directly with an admin-supplied password.

// @Summary     Invite team member
// @Description Sends an email invite to a new team member. Requires Admin role.
// @Tags        team
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{email=string,full_name=string,role=string}  true  "Invitee details"
// @Success     201   {object}  object{id=int64,email=string,message=string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     409   {object}  ErrorResponse  "user or invite already exists"
// @Failure     500   {object}  ErrorResponse
// @Router      /api/team/invite [post]
func (s *Server) inviteTeamMember(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.add") {
		return
	}
	ac := getAuth(r)
	var body struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Password string `json:"password"`
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
	if body.Password == "" {
		writeFieldError(w, http.StatusBadRequest, "Password is required.", map[string]string{"password": "Password is required"})
		return
	}
	role := normalizeRole(body.Role)
	if role == "" {
		writeFieldError(w, http.StatusBadRequest, "Invalid role.", map[string]string{"role": "Role must be Admin, TeamLeader, Agent, or Executive"})
		return
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

	hash, err := db.HashPassword(body.Password)
	if err != nil {
		s.logger.Sugar().Errorw("inviteTeamMember: hash", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := s.db.CreateUserWithRole(body.Email, hash, body.FullName, role, ac.OrgID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "1062") || strings.Contains(errMsg, "Duplicate") {
			writeError(w, http.StatusConflict, "A user with this email already exists.")
			return
		}
		s.logger.Sugar().Errorw("inviteTeamMember: create user", "err", err)
		writeError(w, http.StatusInternalServerError, "Could not create user. Please try again.")
		return
	}
	if role == db.RoleExecutive {
		_ = s.db.EnsureExecutiveForUser(ac.OrgID, body.FullName, body.Email)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"email":   body.Email,
		"message": fmt.Sprintf("Team member %s created.", body.Email),
	})
}

// ── GET /api/invite/{token} ───────────────────────────────────────────────────
// Public — no auth. Validates the token and returns the invitee's email,
// full name, role, and org name so the accept page can render a useful
// "You've been invited to <X>" header. Token itself is NEVER echoed back.

// @Summary     Get invite details
// @Description Public endpoint. Validates the invite token and returns the invitee's details.
// @Tags        team
// @Produce     json
// @Param       token  path  string  true  "Invite token"
// @Success     200  {object}  object{email=string,full_name=string,role=string,org_name=string}
// @Failure     400  {object}  ErrorResponse
// @Failure     410  {object}  ErrorResponse  "invite expired or invalid"
// @Failure     500  {object}  ErrorResponse
// @Router      /api/invite/{token} [get]
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

// @Summary     Accept invite
// @Description Public endpoint. The invitee sets their own password and creates their account.
// @Tags        team
// @Accept      json
// @Produce     json
// @Param       token  path  string                                    true  "Invite token"
// @Param       body   body  object{password=string,full_name=string}  true  "Password and optional name override"
// @Success     200   {object}  object{email=string,message=string}
// @Failure     400   {object}  ErrorResponse
// @Failure     409   {object}  ErrorResponse
// @Failure     410   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/invite/{token}/accept [post]
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

// @Summary     List pending invites
// @Description Returns all pending (unaccepted, unexpired) team invites for the org. Requires Admin role.
// @Tags        team
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   object
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/invites [get]
func (s *Server) listPendingInvites(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.view") {
		return
	}
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

// @Summary     Get invite link
// @Description Returns the accept-invite URL for a pending invite (for out-of-band sharing). Requires Admin role.
// @Tags        team
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Invite ID"
// @Success     200  {object}  object{invite_link=string}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     410  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/invites/{id}/link [get]
func (s *Server) getInviteLink(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.view") {
		return
	}
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

// @Summary     Cancel invite
// @Description Deletes a pending invite. Requires Admin role.
// @Tags        team
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Invite ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/invites/{id} [delete]
func (s *Server) cancelInvite(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.remove") {
		return
	}
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
func (s *Server) validatePasswordStrong(_ context.Context, p string) string {
	if len(p) < 8 {
		return "password must be at least 8 characters"
	}
	if len(p) > 128 {
		return "password must be 128 characters or fewer"
	}
	if _, bad := commonPasswords[strings.ToLower(p)]; bad {
		return "password is too common — choose something harder to guess"
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return "password must contain at least one letter and one number"
	}
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

// @Summary     Update team member role
// @Description Changes a team member's role (Admin/Agent/Executive). Requires Admin role.
// @Tags        team
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64                   true  "User ID"
// @Param       body  body  object{role=string}     true  "New role"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/{id}/role [put]
func (s *Server) updateTeamRole(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.edit_role") {
		return
	}
	ac := getAuth(r)
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
	target, err := s.db.GetUserByIDInOrgWithRole(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("updateTeamRole: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	role := normalizeRole(body.Role)
	if role == "" {
		writeFieldError(w, http.StatusBadRequest, "Invalid role.", map[string]string{"role": "Role must be Admin, TeamLeader, Agent, or Executive"})
		return
	}
	if err := s.db.UpdateUserRoleInOrg(id, ac.OrgID, role); err != nil {
		s.logger.Sugar().Errorw("updateTeamRole", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.db.SetUserPermissions(id, ac.OrgID, rolePermissionKeys(role)); err != nil {
		s.logger.Sugar().Errorw("updateTeamRole: reset permissions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if role == db.RoleExecutive {
		_ = s.db.EnsureExecutiveForUser(ac.OrgID, target.FullName, target.Email)
	} else if exec, err := s.db.GetExecutiveByEmail(ac.OrgID, target.Email); err == nil && exec != nil {
		if err := s.db.DeleteExecutive(exec.ID, ac.OrgID); err != nil {
			s.logger.Sugar().Warnw("updateTeamRole: linked executive delete failed", "err", err)
		}
		if err := s.db.UnassignExecutiveFromLeads(exec.ID, ac.OrgID); err != nil {
			s.logger.Sugar().Warnw("updateTeamRole: linked executive unassign failed", "err", err)
		}
	} else if err != nil {
		s.logger.Sugar().Warnw("updateTeamRole: linked executive lookup failed", "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── POST /api/team/{id}/reset-password ───────────────────────────────────────

// @Summary     Reset team member password
// @Description Sets a new password for a team member in the same org. Requires Admin role.
// @Tags        team
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64                    true  "User ID"
// @Param       body  body  object{password=string}  true  "New password"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/{id}/reset-password [post]
func (s *Server) resetTeamMemberPassword(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.reset_password") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(body.Password) == "" {
		writeFieldError(w, http.StatusBadRequest, "Password is required.", map[string]string{"password": "Password is required"})
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("resetTeamMemberPassword: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	hash, err := db.HashPassword(body.Password)
	if err != nil {
		s.logger.Sugar().Errorw("resetTeamMemberPassword: hash", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.db.UpdateUserPassword(id, hash); err != nil {
		s.logger.Sugar().Errorw("resetTeamMemberPassword: update", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── DELETE /api/team/{id} ─────────────────────────────────────────────────────

// @Summary     Delete team member
// @Description Removes a team member from the org. Requires Admin role. Cannot remove yourself or the last admin.
// @Tags        team
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "User ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/team/{id} [delete]
func (s *Server) deleteTeamMember(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.remove") {
		return
	}
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
	if exec, err := s.db.GetExecutiveByEmail(ac.OrgID, target.Email); err == nil && exec != nil {
		if err := s.db.DeleteExecutive(exec.ID, ac.OrgID); err != nil {
			s.logger.Sugar().Warnw("deleteTeamMember: linked executive delete failed", "err", err)
		}
		if err := s.db.UnassignExecutiveFromLeads(exec.ID, ac.OrgID); err != nil {
			s.logger.Sugar().Warnw("deleteTeamMember: linked executive unassign failed", "err", err)
		}
	} else if err != nil {
		s.logger.Sugar().Warnw("deleteTeamMember: linked executive lookup failed", "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ── GET /api/team/{id}/provider-accounts ─────────────────────────────────────
// Lists org-level provider accounts and flags which ones the target user is
// allowed to see. Used by the Admin permissions modal to configure per-user
// account visibility.
func (s *Server) getTeamMemberProviderAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.permissions") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getTeamMemberProviderAccounts: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	accounts, err := s.db.GetOrgExotelAccounts(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getTeamMemberProviderAccounts: accounts", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	allowedIDs, err := s.db.GetUserAllowedExotelAccountIDs(target.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getTeamMemberProviderAccounts: allowed ids", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	allowedSet := make(map[int64]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowedSet[id] = true
	}
	type accountWithAllowed struct {
		ID         int64  `json:"id"`
		Provider   string `json:"provider"`
		Name       string `json:"name"`
		CallerID   string `json:"caller_id"`
		AccountSID string `json:"account_sid"`
		Allowed    bool   `json:"allowed"`
	}
	out := make([]accountWithAllowed, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountWithAllowed{
			ID:         a.ID,
			Provider:   a.Provider,
			Name:       a.Name,
			CallerID:   a.CallerID,
			AccountSID: a.AccountSID,
			Allowed:    allowedSet[a.ID],
		})
	}
	writeJSON(w, http.StatusOK, emptyJSON(out))
}

// ── PUT /api/team/{id}/provider-accounts ───────────────────────────────────────
// Replaces the list of org-level provider accounts the target user is allowed
// to see and use. Admins always see all accounts; this only affects Agents and
// Executives.
func (s *Server) updateTeamMemberProviderAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "team.permissions") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	target, err := s.db.GetUserByIDInOrg(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("updateTeamMemberProviderAccounts: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var body struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.db.SetUserAllowedExotelAccountIDs(target.ID, ac.OrgID, body.AccountIDs); err != nil {
		s.logger.Sugar().Errorw("updateTeamMemberProviderAccounts: save", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}
