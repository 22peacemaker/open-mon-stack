package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

const SessionCookie = "oms_session"
const SessionKey = "session"
const TokenHeader = "X-OMS-Token"

// RequireAuth validates the session cookie and sets the session in the Echo context.
func RequireAuth(store *storage.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess, err := resolveSession(c, store)
			if err != nil {
				return err
			}
			c.Set(SessionKey, sess)
			return next(c)
		}
	}
}

// RequireAdmin validates the session cookie and checks that the user has the admin role.
func RequireAdmin(store *storage.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess, err := resolveSession(c, store)
			if err != nil {
				return err
			}
			if sess.Role != models.RoleAdmin {
				return echo.NewHTTPError(http.StatusForbidden, "admin access required")
			}
			c.Set(SessionKey, sess)
			return next(c)
		}
	}
}

func resolveSession(c echo.Context, store *storage.Store) (*models.Session, error) {
	if raw := c.Request().Header.Get(TokenHeader); raw != "" {
		token, ok := store.VerifyToken(raw)
		if !ok {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
		}
		return &models.Session{
			ID:        "",
			UserID:    "",
			Username:  "token",
			Role:      token.Role,
			ExpiresAt: time.Time{},
		}, nil
	}
	cookie, err := c.Cookie(SessionCookie)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	sess, ok := store.GetSession(cookie.Value)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired session")
	}
	return sess, nil
}
