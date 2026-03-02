package models

import "time"

// APIToken is a static token for programmatic access (CI/CD, scripts).
// The raw token value is only returned once on creation; only TokenHash is stored.
type APIToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name,omitempty"`
	Role      Role       `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	TokenHash string     `json:"token_hash"` // not exposed in API responses
	CreatedAt time.Time  `json:"created_at"`
}
