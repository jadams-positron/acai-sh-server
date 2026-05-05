package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"

	"github.com/acai-sh/server/internal/store"
)

// SessionLifetime is how long a session cookie persists if remember-me is set.
const SessionLifetime = 14 * 24 * time.Hour

// NewSessionManager returns a configured *scs.SessionManager backed by SQLite.
// secureCookie should be true in prod (HTTPS) and false in dev.
func NewSessionManager(db *store.DB, secureCookie bool) *scs.SessionManager {
	mgr := scs.New()
	mgr.Store = sqlite3store.New(db.Write)
	mgr.Lifetime = SessionLifetime
	mgr.IdleTimeout = 0
	mgr.Cookie.Name = "_acai_session"
	mgr.Cookie.Path = "/"
	mgr.Cookie.HttpOnly = true
	mgr.Cookie.Secure = secureCookie
	mgr.Cookie.SameSite = http.SameSiteLaxMode
	mgr.Cookie.Persist = false // remember-me toggling lands in P1c
	return mgr
}

// Session keys.
const (
	sessionKeyUserID          = "user_id"
	sessionKeyAuthenticatedAt = "authenticated_at"
)

// Login sets the session keys for an authenticated user.
func Login(ctx context.Context, mgr *scs.SessionManager, userID string) {
	mgr.Put(ctx, sessionKeyUserID, userID)
	mgr.Put(ctx, sessionKeyAuthenticatedAt, time.Now().UTC().Format(time.RFC3339Nano))
}

// Logout clears the session.
func Logout(ctx context.Context, mgr *scs.SessionManager) error {
	return mgr.Destroy(ctx)
}

// CurrentUserID returns the user id from the session, or "" if anonymous.
func CurrentUserID(ctx context.Context, mgr *scs.SessionManager) string {
	return mgr.GetString(ctx, sessionKeyUserID)
}
