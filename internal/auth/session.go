package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v4"
)

const (
	sessionName = "_acai_session"

	// SessionLifetime is how long a session cookie persists.
	SessionLifetime = 14 * 24 * time.Hour
)

// SessionStore wraps a *sessions.CookieStore configured for Acai.
type SessionStore struct {
	*sessions.CookieStore
}

// NewSessionStore constructs a CookieStore signed (and encrypted) with
// keys derived from secretKeyBase. secureCookie should be true in prod.
func NewSessionStore(secretKeyBase string, secureCookie bool) *SessionStore {
	authKey := []byte(secretKeyBase)
	encKey := deriveEncKey(authKey) // 32-byte enc key derived from auth key
	store := sessions.NewCookieStore(authKey, encKey)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(SessionLifetime.Seconds()),
		Secure:   secureCookie,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	return &SessionStore{CookieStore: store}
}

// SessionFromEcho returns the active session for the request (creating it on
// the cookie store side if absent).
func (s *SessionStore) SessionFromEcho(c echo.Context) *sessions.Session {
	sess, _ := s.Get(c.Request(), sessionName)
	return sess
}

// Login records user_id + authenticated_at in the session and saves it.
func (s *SessionStore) Login(c echo.Context, userID string) error {
	sess := s.SessionFromEcho(c)
	sess.Values["user_id"] = userID
	sess.Values["authenticated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return sess.Save(c.Request(), c.Response())
}

// Logout clears the session cookie.
func (s *SessionStore) Logout(c echo.Context) error {
	sess := s.SessionFromEcho(c)
	sess.Options.MaxAge = -1
	for k := range sess.Values {
		delete(sess.Values, k)
	}
	return sess.Save(c.Request(), c.Response())
}

// CurrentUserID returns the user id from the session, or "" if anonymous.
func (s *SessionStore) CurrentUserID(c echo.Context) string {
	sess := s.SessionFromEcho(c)
	if v, ok := sess.Values["user_id"].(string); ok {
		return v
	}
	return ""
}

func deriveEncKey(authKey []byte) []byte {
	mac := hmac.New(sha256.New, authKey)
	mac.Write([]byte("acai-session-enc-v1"))
	sum := mac.Sum(nil)
	out := make([]byte, 32)
	copy(out, sum)
	return out
}
