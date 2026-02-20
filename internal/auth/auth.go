package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"luna/internal/models"

	"gorm.io/gorm"
)

type contextKey string

const UserContextKey contextKey = "user"

const SessionCookieName = "luna_session"
const SessionDuration = 30 * 24 * time.Hour

type Middleware struct {
	db *gorm.DB
}

func NewMiddleware(db *gorm.DB) *Middleware {
	return &Middleware{db: db}
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := m.attachUser(r.Context(), r)
		if GetUser(ctx) == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) attachUser(ctx context.Context, r *http.Request) context.Context {
	user := m.getSessionUser(r)
	return context.WithValue(ctx, UserContextKey, user)
}

func (m *Middleware) getSessionUser(r *http.Request) *models.User {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}

	var session models.Session
	if err := m.db.Preload("User").
		Where("id = ? AND expires_at > ?", cookie.Value, time.Now().UTC()).
		First(&session).Error; err != nil {
		return nil
	}

	user := session.User
	if !user.IsActive {
		return nil
	}
	return &user
}

func NewSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read random salt: %w", err)
	}

	dk := deriveKey(password, salt, 120000)
	return fmt.Sprintf("v1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(dk)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "v1" {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	actual := deriveKey(password, salt, 120000)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func deriveKey(password string, salt []byte, rounds int) []byte {
	input := make([]byte, len(password)+len(salt))
	copy(input, []byte(password))
	copy(input[len(password):], salt)
	sum := sha256.Sum256(input)
	digest := sum[:]

	for i := 1; i < rounds; i++ {
		next := sha256.Sum256(digest)
		digest = next[:]
	}
	out := make([]byte, len(digest))
	copy(out, digest)
	return out
}

func GetUser(ctx context.Context) *models.User {
	if user, ok := ctx.Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}
