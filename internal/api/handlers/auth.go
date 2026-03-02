package handlers

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	authmw "github.com/22peacemaker/open-mon-stack/internal/api/middleware"
	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

type AuthHandler struct {
	store *storage.Store
}

func NewAuthHandler(store *storage.Store) *AuthHandler {
	return &AuthHandler{store: store}
}

// SetupStatus returns whether first-run setup is required.
func (h *AuthHandler) SetupStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{
		"needs_setup": h.store.UserCount() == 0,
	})
}

// Setup creates the first admin account. Fails if any user already exists.
func (h *AuthHandler) Setup(c echo.Context) error {
	if h.store.UserCount() > 0 {
		return echo.NewHTTPError(http.StatusConflict, "setup already completed")
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username is required")
	}
	if len(req.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash password")
	}
	user := &models.User{
		ID:           newID(),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	}
	if err := h.store.AddUser(user); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save user")
	}
	return h.loginUser(c, user)
}

// Login authenticates a user and issues a session cookie.
func (h *AuthHandler) Login(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	user, ok := h.store.GetUserByUsername(req.Username)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
	}
	return h.loginUser(c, user)
}

// Logout clears the session cookie and removes the server-side session.
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(authmw.SessionCookie)
	if err == nil {
		h.store.DeleteSession(cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name:     authmw.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return c.NoContent(http.StatusNoContent)
}

// Me returns the currently authenticated user.
func (h *AuthHandler) Me(c echo.Context) error {
	sess := c.Get(authmw.SessionKey).(*models.Session)
	return c.JSON(http.StatusOK, map[string]string{
		"id":       sess.UserID,
		"username": sess.Username,
		"role":     string(sess.Role),
	})
}

// ChangePassword lets any authenticated user change their own password.
// Requires the current password for verification; invalidates the active session afterwards.
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	sess := c.Get(authmw.SessionKey).(*models.Session)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current_password and new_password are required")
	}
	if len(req.NewPassword) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be at least 8 characters")
	}

	user, ok := h.store.GetUser(sess.UserID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash password")
	}
	updated := *user
	updated.PasswordHash = string(hash)
	if err := h.store.UpdateUser(&updated); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update password")
	}

	// Invalidate the current session so the user must log in again
	cookie, err := c.Cookie(authmw.SessionCookie)
	if err == nil {
		h.store.DeleteSession(cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name:     authmw.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return c.NoContent(http.StatusNoContent)
}

// loginUser creates a session and sets the session cookie, then returns the user info.
func (h *AuthHandler) loginUser(c echo.Context, user *models.User) error {
	sess := h.store.CreateSession(user.ID, user.Username, user.Role)
	c.SetCookie(&http.Cookie{
		Name:     authmw.SessionCookie,
		Value:    sess.ID,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return c.JSON(http.StatusOK, map[string]string{
		"id":       user.ID,
		"username": user.Username,
		"role":     string(user.Role),
	})
}

