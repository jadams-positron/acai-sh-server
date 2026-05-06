package testfx

import (
	"encoding/gob"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/securecookie"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

func init() {
	// gorilla/sessions encodes session.Values via gob. Register the concrete
	// map type so the decoder can reconstruct it on the server side.
	gob.Register(map[any]any{})
}

// LoggedInClient returns a Client with a valid session cookie for the given
// user. It bypasses the magic-link flow by constructing a signed+encrypted
// securecookie that matches what the server expects.
func LoggedInClient(t *testing.T, app *App, user *accounts.User) *Client {
	t.Helper()

	authKey := []byte(app.Cfg.SecretKeyBase)
	encKey := auth.DeriveSessionEncKey(authKey)

	sc := securecookie.New(authKey, encKey)
	sc.SetSerializer(securecookie.GobEncoder{})

	values := map[any]any{
		"user_id":          user.ID,
		"authenticated_at": time.Now().UTC().Format(time.RFC3339Nano),
	}

	encoded, err := sc.Encode("_acai_session", values)
	if err != nil {
		t.Fatalf("testfx.LoggedInClient: encode session: %v", err)
	}

	cookie := &http.Cookie{ //nolint:gosec // G124: test-only cookie; no TLS in tests, Secure=false is intentional
		Name:     "_acai_session",
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
	}
	return HTTPClient(t, app.Echo).WithHeader("Cookie", cookie.String())
}
