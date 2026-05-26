package api

// ErrorResponse is returned when a request fails.
type ErrorResponse struct {
	Error string `json:"error"`
}

// MessageResponse is returned for success messages.
type MessageResponse struct {
	Message string `json:"message"`
}

// IDResponse is returned when a resource is created.
type IDResponse struct {
	ID int64 `json:"id"`
}

// DeletedResponse is returned after a successful delete.
type DeletedResponse struct {
	Deleted bool `json:"deleted"`
}

// BoolResponse is a generic boolean result envelope.
type BoolResponse struct {
	OK bool `json:"ok"`
}

// TokenResponse is returned by auth login/signup.
type TokenResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	User        UserResponse `json:"user"`
}

// UserResponse is the public user profile shape.
type UserResponse struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	OrgID    int64  `json:"org_id"`
	OrgName  string `json:"org_name"`
}

// SSETicketResponse is returned by GET /api/sse/ticket.
type SSETicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
}
