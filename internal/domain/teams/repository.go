package teams

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Repository owns team and access-token persistence.
type Repository struct {
	db *store.DB
}

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// ErrInvalidToken is returned when token verification fails for any reason.
var ErrInvalidToken = errors.New("teams: invalid token")

// IsInvalidToken reports whether err is or wraps ErrInvalidToken.
func IsInvalidToken(err error) bool { return errors.Is(err, ErrInvalidToken) }

// CreateTeam inserts a new team with the given name.
func (r *Repository) CreateTeam(ctx context.Context, name string) (*Team, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("teams: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := sqlc.New(r.db.Write)
	row, err := q.CreateTeam(ctx, sqlc.CreateTeamParams{
		ID:          id.String(),
		Name:        name,
		GlobalAdmin: 0,
		InsertedAt:  now,
		UpdatedAt:   now,
	})
	if err != nil {
		return nil, fmt.Errorf("teams: insert: %w", err)
	}
	return teamFromRow(row)
}

// GetTeamByID returns the team or an error.
func (r *Repository) GetTeamByID(ctx context.Context, id string) (*Team, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetTeamByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("teams: GetTeamByID: %w", err)
	}
	return teamFromRow(row)
}

// CreateAccessTokenParams is the input for CreateAccessToken.
type CreateAccessTokenParams struct {
	UserID    string
	TeamID    string
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// CreateAccessToken generates a new bearer token, stores its argon2id hash,
// and returns the plaintext token. Format: "<prefix>.<secret>".
func (r *Repository) CreateAccessToken(ctx context.Context, p CreateAccessTokenParams) (string, error) {
	prefix, err := randomURLSafe(6) // ~8 url-safe-b64 chars
	if err != nil {
		return "", err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("teams: gen secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	plaintext := prefix + "." + secret

	hash, err := auth.HashAccessSecret(secret, prefix)
	if err != nil {
		return "", fmt.Errorf("teams: hash: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("teams: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	scopesJSON, err := json.Marshal(p.Scopes)
	if err != nil {
		return "", fmt.Errorf("teams: marshal scopes: %w", err)
	}

	var expiresAt *string
	if p.ExpiresAt != nil {
		s := p.ExpiresAt.UTC().Format(time.RFC3339Nano)
		expiresAt = &s
	}

	q := sqlc.New(r.db.Write)
	if _, err := q.CreateAccessToken(ctx, sqlc.CreateAccessTokenParams{
		ID:          id.String(),
		UserID:      p.UserID,
		TeamID:      p.TeamID,
		Name:        p.Name,
		TokenHash:   hash,
		TokenPrefix: prefix,
		Scopes:      string(scopesJSON),
		ExpiresAt:   expiresAt,
		RevokedAt:   nil,
		LastUsedAt:  nil,
		InsertedAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		return "", fmt.Errorf("teams: insert access_token: %w", err)
	}

	return plaintext, nil
}

// VerifyAccessToken validates the plaintext token, returning the access token
// row and the team it belongs to.
func (r *Repository) VerifyAccessToken(ctx context.Context, plaintext string) (*AccessToken, *Team, error) {
	prefix, secret, ok := strings.Cut(plaintext, ".")
	if !ok || prefix == "" || secret == "" {
		return nil, nil, ErrInvalidToken
	}

	q := sqlc.New(r.db.Read)
	row, err := q.GetAccessTokenByPrefix(ctx, prefix)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	if err := auth.VerifyAccessSecret(secret, prefix, row.TokenHash); err != nil {
		return nil, nil, ErrInvalidToken
	}

	tok := accessTokenFromRow(row)
	if !tok.IsValid(time.Now()) {
		return nil, nil, ErrInvalidToken
	}

	team, err := r.GetTeamByID(ctx, row.TeamID)
	if err != nil {
		return nil, nil, fmt.Errorf("teams: load team: %w", err)
	}

	// Fire-and-forget last_used_at update. Uses a fresh context because the
	// request context will be canceled before the goroutine completes.
	go func() { //nolint:gosec // G118: intentional — request context would be canceled
		bgctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_ = sqlc.New(r.db.Write).UpdateAccessTokenLastUsed(bgctx, sqlc.UpdateAccessTokenLastUsedParams{
			LastUsedAt: &now,
			UpdatedAt:  now,
			ID:         tok.ID,
		})
	}()

	return tok, team, nil
}

// RevokeAccessTokenByPrefix sets revoked_at = now() for the matching token.
func (r *Repository) RevokeAccessTokenByPrefix(ctx context.Context, prefix string) error {
	q := sqlc.New(r.db.Read)
	row, err := q.GetAccessTokenByPrefix(ctx, prefix)
	if err != nil {
		return fmt.Errorf("teams: GetAccessTokenByPrefix: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	wq := sqlc.New(r.db.Write)
	if err := wq.RevokeAccessToken(ctx, sqlc.RevokeAccessTokenParams{
		RevokedAt: &now,
		UpdatedAt: now,
		ID:        row.ID,
	}); err != nil {
		return fmt.Errorf("teams: RevokeAccessToken: %w", err)
	}
	return nil
}

// --- helpers ---

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("teams: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func teamFromRow(row sqlc.Team) (*Team, error) {
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	return &Team{
		ID:          row.ID,
		Name:        row.Name,
		GlobalAdmin: row.GlobalAdmin != 0,
		InsertedAt:  insertedAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func accessTokenFromRow(row sqlc.AccessToken) *AccessToken {
	t := &AccessToken{
		ID:          row.ID,
		UserID:      row.UserID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
	}
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	t.InsertedAt = insertedAt
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	t.UpdatedAt = updatedAt

	if err := json.Unmarshal([]byte(row.Scopes), &t.Scopes); err != nil {
		t.Scopes = nil
	}

	if row.ExpiresAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.ExpiresAt)
		if err == nil {
			t.ExpiresAt = &ts
		}
	}
	if row.RevokedAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.RevokedAt)
		if err == nil {
			t.RevokedAt = &ts
		}
	}
	if row.LastUsedAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.LastUsedAt)
		if err == nil {
			t.LastUsedAt = &ts
		}
	}
	return t
}
