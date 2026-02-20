package auth

import (
	"context"
	"net/http"

	"luna/internal/models"

	"gorm.io/gorm"
)

type contextKey string

const UserContextKey contextKey = "user"

type Middleware struct {
	db *gorm.DB
}

func NewMiddleware(db *gorm.DB) *Middleware {
	return &Middleware{db: db}
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := m.attachUser(r.Context(), r)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) attachUser(ctx context.Context, r *http.Request) context.Context {
	user := m.getDevUser(ctx)
	return context.WithValue(ctx, UserContextKey, user)
}

func (m *Middleware) getDevUser(ctx context.Context) *models.User {
	// TODO: Implement real auth
	// - Check for session cookie or JWT token
	// - Validate user exists
	// - Return actual user from database

	// Dev mode: return mock user
	return &models.User{
		ID:       1,
		Username: "devuser",
		Role:     models.RoleAdmin,
	}
}

func GetUser(ctx context.Context) *models.User {
	if user, ok := ctx.Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}
