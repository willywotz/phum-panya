package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// sessionCookieName is the name of the session cookie. There is no
// separate CSRF cookie: SameSite=Strict is the CSRF defense.
const sessionCookieName = "session"

// ErrNoSession is returned when a session token is missing or expired.
var ErrNoSession = errors.New("no valid session")

// Sessions manages server-side login sessions.
type Sessions interface {
	Create(userID uint) (rawToken string, err error)
	Lookup(rawToken string) (userID uint, err error)
	Delete(rawToken string) error
}

// SessionStore manages server-side login sessions backed by GORM.
type SessionStore struct {
	db  *gorm.DB
	clk clock.Clock
	ttl time.Duration
}

// NewSessionStore returns a SessionStore that expires sessions after ttl.
func NewSessionStore(g *gorm.DB, clk clock.Clock, ttl time.Duration) *SessionStore {
	return &SessionStore{db: g, clk: clk, ttl: ttl}
}

// Create starts a new session for userID and returns the raw token.
// Only the hash of the token is stored.
func (s *SessionStore) Create(userID uint) (rawToken string, err error) {
	rawToken, err = randToken()
	if err != nil {
		return "", err
	}
	session := model.Session{
		TokenHash: hashToken(rawToken),
		UserID:    userID,
		ExpiresAt: s.clk.Now().Add(s.ttl),
	}
	if err := s.db.Create(&session).Error; err != nil {
		return "", err
	}
	return rawToken, nil
}

// Lookup returns the user id for rawToken, or ErrNoSession if the
// token is missing or expired.
func (s *SessionStore) Lookup(rawToken string) (userID uint, err error) {
	var session model.Session
	err = s.db.Where("token_hash = ?", hashToken(rawToken)).First(&session).Error
	if err != nil {
		return 0, ErrNoSession
	}
	if !session.ExpiresAt.After(s.clk.Now()) {
		return 0, ErrNoSession
	}
	return session.UserID, nil
}

// Delete removes the session for rawToken.
func (s *SessionStore) Delete(rawToken string) error {
	return s.db.Where("token_hash = ?", hashToken(rawToken)).Delete(&model.Session{}).Error
}

// SetSessionCookie sets the session cookie on the response.
func SetSessionCookie(c *gin.Context, rawToken string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// randToken generates a 32-byte random token, base64url-encoded.
func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken returns the hex-encoded SHA-256 hash of raw.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
