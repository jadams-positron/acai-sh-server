package auth

import (
	"context"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

// MagicLinkValidity is the duration for which a magic-link login token is valid.
const MagicLinkValidity = 15 * time.Minute

// MagicLinkService generates and consumes one-time login URLs backed by the
// accounts repository's email-token primitives.
type MagicLinkService struct {
	repo    *accounts.Repository
	baseURL string
}

// NewMagicLinkService constructs a MagicLinkService that stores tokens via repo
// and builds login URLs rooted at baseURL (e.g. "https://acai.test").
func NewMagicLinkService(repo *accounts.Repository, baseURL string) *MagicLinkService {
	return &MagicLinkService{repo: repo, baseURL: baseURL}
}

// GenerateLoginURL creates a one-time login token for user and returns the
// full login URL and the raw token. The token is valid for MagicLinkValidity.
func (s *MagicLinkService) GenerateLoginURL(ctx context.Context, user *accounts.User) (url, rawToken string, err error) {
	rawToken, err = s.repo.BuildEmailToken(ctx, user, "login")
	if err != nil {
		return "", "", err
	}
	url = s.baseURL + "/users/log-in/" + rawToken
	return url, rawToken, nil
}

// ConsumeLoginToken validates rawToken, deletes it, and returns the associated
// User. Returns an error if the token is not found, already used, or expired.
func (s *MagicLinkService) ConsumeLoginToken(ctx context.Context, rawToken string) (*accounts.User, error) {
	return s.repo.ConsumeEmailToken(ctx, rawToken, "login", MagicLinkValidity)
}
