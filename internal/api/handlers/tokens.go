package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

const tokenSecretBytes = 32

type TokenHandler struct {
	store *storage.Store
}

func NewTokenHandler(store *storage.Store) *TokenHandler {
	return &TokenHandler{store: store}
}

// tokenResponse is the API response for a token (never includes TokenHash or raw token value).
type tokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name,omitempty"`
	Role      models.Role `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func toTokenResponse(t *models.APIToken) tokenResponse {
	return tokenResponse{
		ID:        t.ID,
		Name:      t.Name,
		Role:      t.Role,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
	}
}

// createTokenResponse includes the raw token value (only returned once on creation).
type createTokenResponse struct {
	Token     string     `json:"token"`
	ID        string     `json:"id"`
	Name      string     `json:"name,omitempty"`
	Role      models.Role `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// List returns all API tokens (without secret or hash).
func (h *TokenHandler) List(c echo.Context) error {
	tokens := h.store.ListTokens()
	result := make([]tokenResponse, 0, len(tokens))
	for _, t := range tokens {
		result = append(result, toTokenResponse(t))
	}
	return c.JSON(http.StatusOK, result)
}

// Create generates a new API token and returns the raw value once.
func (h *TokenHandler) Create(c echo.Context) error {
	var req struct {
		Name      string      `json:"name"`
		Role      models.Role `json:"role"`
		ExpiresAt *time.Time  `json:"expires_at"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleViewer {
		return echo.NewHTTPError(http.StatusBadRequest, "role must be 'admin' or 'viewer'")
	}
	id := newID()
	secretBytes := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(secretBytes); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}
	secret := hex.EncodeToString(secretBytes)
	rawToken := "oms_" + id + "_" + secret
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash token")
	}
	now := time.Now()
	t := &models.APIToken{
		ID:        id,
		Name:      req.Name,
		Role:      req.Role,
		ExpiresAt: req.ExpiresAt,
		TokenHash: string(hash),
		CreatedAt: now,
	}
	if err := h.store.AddToken(t); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save token")
	}
	return c.JSON(http.StatusCreated, createTokenResponse{
		Token:     rawToken,
		ID:        t.ID,
		Name:      t.Name,
		Role:      t.Role,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
	})
}

// Delete revokes a token by ID.
func (h *TokenHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if _, ok := h.store.GetToken(id); !ok {
		return echo.NewHTTPError(http.StatusNotFound, "token not found")
	}
	if err := h.store.DeleteToken(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete token")
	}
	return c.NoContent(http.StatusNoContent)
}
