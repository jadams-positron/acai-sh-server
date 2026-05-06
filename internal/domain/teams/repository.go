package teams

import (
	"context"
	"crypto/rand"
	"database/sql"
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

// ErrNotFound is returned when no matching team exists.
var ErrNotFound = errors.New("teams: not found")

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// ErrInvalidToken is returned when token verification fails for any reason.
var ErrInvalidToken = errors.New("teams: invalid token")

// ErrDuplicateName is returned when a team name is already taken.
var ErrDuplicateName = errors.New("teams: duplicate team name")

// IsDuplicateName reports whether err is or wraps ErrDuplicateName.
func IsDuplicateName(err error) bool { return errors.Is(err, ErrDuplicateName) }

// ErrInvalidTeamName is returned for malformed team names.
var ErrInvalidTeamName = errors.New("teams: invalid team name (must be alphanumeric + hyphens/underscores, 1-64 chars)")

// IsInvalidTeamName reports whether err is or wraps ErrInvalidTeamName.
func IsInvalidTeamName(err error) bool { return errors.Is(err, ErrInvalidTeamName) }

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

// ListForUser returns all teams the user belongs to via user_team_roles.
func (r *Repository) ListForUser(ctx context.Context, userID string) ([]*Team, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListTeamsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("teams: ListForUser: %w", err)
	}
	out := make([]*Team, 0, len(rows))
	for _, row := range rows {
		t, err := teamFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// CreateTeamWithOwner atomically creates a team and links userID as the
// "owner" role via user_team_roles. Validates name format before inserting.
func (r *Repository) CreateTeamWithOwner(ctx context.Context, userID, name string) (*Team, error) {
	if err := validateTeamName(name); err != nil {
		return nil, err
	}
	team, err := r.CreateTeam(ctx, name)
	if err != nil {
		// Surface SQLite UNIQUE constraint violations as ErrDuplicateName.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateName, name)
		}
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := sqlc.New(r.db.Write)
	if err := q.CreateUserTeamRole(ctx, sqlc.CreateUserTeamRoleParams{
		TeamID:     team.ID,
		UserID:     userID,
		Title:      "owner",
		InsertedAt: now,
		UpdatedAt:  now,
	}); err != nil {
		return nil, fmt.Errorf("teams: link owner: %w", err)
	}
	return team, nil
}

func validateTeamName(name string) error {
	if len(name) < 1 || len(name) > 64 {
		return fmt.Errorf("%w: length %d", ErrInvalidTeamName, len(name))
	}
	for _, c := range name {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return fmt.Errorf("%w: invalid character %q", ErrInvalidTeamName, c)
		}
	}
	return nil
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

// GetByName returns the team with the given name (case-insensitive), or
// ErrNotFound. The user's authorization to view the team is checked separately.
func (r *Repository) GetByName(ctx context.Context, name string) (*Team, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetTeamByName(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("teams: GetByName: %w", err)
	}
	return teamFromRow(row)
}

// Member is one row of a team's membership listing.
type Member struct {
	UserID   string
	Email    string
	Role     string
	JoinedAt time.Time
}

// ListMembers returns all members of teamID ordered by join date.
func (r *Repository) ListMembers(ctx context.Context, teamID string) ([]*Member, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListMembersForTeam(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("teams: ListMembers: %w", err)
	}
	out := make([]*Member, 0, len(rows))
	for _, row := range rows {
		joinedAt, _ := time.Parse(time.RFC3339Nano, row.JoinedAt)
		out = append(out, &Member{
			UserID:   row.UserID,
			Email:    row.UserEmail,
			Role:     row.RoleTitle,
			JoinedAt: joinedAt,
		})
	}
	return out, nil
}

// IsMember reports whether userID has a user_team_roles entry for teamID.
func (r *Repository) IsMember(ctx context.Context, teamID, userID string) (bool, error) {
	q := sqlc.New(r.db.Read)
	_, err := q.GetUserTeamRole(ctx, sqlc.GetUserTeamRoleParams{
		TeamID: teamID,
		UserID: userID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("teams: IsMember: %w", err)
	}
	return true, nil
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
