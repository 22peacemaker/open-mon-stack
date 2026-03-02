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

type UserHandler struct {
	store *storage.Store
}

func NewUserHandler(store *storage.Store) *UserHandler {
	return &UserHandler{store: store}
}

// userResponse strips the password hash before returning to the client.
type userResponse struct {
	ID        string     `json:"id"`
	Username  string     `json:"username"`
	Role      models.Role `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}

func toUserResponse(u *models.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt}
}

func (h *UserHandler) List(c echo.Context) error {
	users := h.store.ListUsers()
	result := make([]userResponse, 0, len(users))
	for _, u := range users {
		result = append(result, toUserResponse(u))
	}
	return c.JSON(http.StatusOK, result)
}

func (h *UserHandler) Get(c echo.Context) error {
	u, ok := h.store.GetUser(c.Param("id"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return c.JSON(http.StatusOK, toUserResponse(u))
}

func (h *UserHandler) Create(c echo.Context) error {
	var req struct {
		Username string      `json:"username"`
		Password string      `json:"password"`
		Role     models.Role `json:"role"`
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
	if req.Role != models.RoleAdmin && req.Role != models.RoleViewer {
		return echo.NewHTTPError(http.StatusBadRequest, "role must be 'admin' or 'viewer'")
	}
	if _, exists := h.store.GetUserByUsername(req.Username); exists {
		return echo.NewHTTPError(http.StatusConflict, "username already taken")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash password")
	}
	user := &models.User{
		ID:           newID(),
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         req.Role,
		CreatedAt:    time.Now(),
	}
	if err := h.store.AddUser(user); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save user")
	}
	return c.JSON(http.StatusCreated, toUserResponse(user))
}

func (h *UserHandler) Update(c echo.Context) error {
	id := c.Param("id")
	existing, ok := h.store.GetUser(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var req struct {
		Role     models.Role `json:"role"`
		Password string      `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	updated := *existing

	if req.Role != "" {
		if req.Role != models.RoleAdmin && req.Role != models.RoleViewer {
			return echo.NewHTTPError(http.StatusBadRequest, "role must be 'admin' or 'viewer'")
		}
		// Prevent demoting the last admin
		if existing.Role == models.RoleAdmin && req.Role == models.RoleViewer && h.store.AdminCount() <= 1 {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot demote the last admin")
		}
		updated.Role = req.Role
	}

	if req.Password != "" {
		if len(req.Password) < 8 {
			return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash password")
		}
		updated.PasswordHash = string(hash)
	}

	if err := h.store.UpdateUser(&updated); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update user")
	}
	return c.JSON(http.StatusOK, toUserResponse(&updated))
}

func (h *UserHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	sess := c.Get(authmw.SessionKey).(*models.Session)

	if id == sess.UserID {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete your own account")
	}
	existing, ok := h.store.GetUser(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	if existing.Role == models.RoleAdmin && h.store.AdminCount() <= 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete the last admin")
	}
	if err := h.store.DeleteUser(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete user")
	}
	return c.NoContent(http.StatusNoContent)
}
