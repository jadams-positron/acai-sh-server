package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// ErrNotFound is returned by Repository methods when a requested record does
// not exist.
var ErrNotFound = errors.New("accounts: not found")

// IsNotFound reports whether err is (or wraps) ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// CreateUserParams carries the input fields for creating a new user.
type CreateUserParams struct {
	Email          string
	HashedPassword string // empty means no password set
}

// Repository provides read/write access to account-related records.
type Repository struct {
	db *store.DB
}

// NewRepository constructs a Repository backed by the given store.DB.
func NewRepository(db *store.DB) *Repository {
	return &Repository{db: db}
}

// CreateUser inserts a new user and returns the domain-mapped value.
func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("accounts.CreateUser: generate id: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	var hashedPassword *string
	if params.HashedPassword != "" {
		hashedPassword = &params.HashedPassword
	}

	q := sqlc.New(r.db.Write)
	row, err := q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:             id.String(),
		Email:          params.Email,
		HashedPassword: hashedPassword,
		ConfirmedAt:    nil,
		InsertedAt:     now,
		UpdatedAt:      now,
	})
	if err != nil {
		return nil, fmt.Errorf("accounts.CreateUser: %w", err)
	}

	return userFromRow(row)
}

// GetUserByEmail looks up a user by email address (case-insensitive).
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("accounts.GetUserByEmail: %w", err)
	}
	return userFromRow(row)
}

// GetUserByID looks up a user by its primary key.
func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("accounts.GetUserByID: %w", err)
	}
	return userFromRow(row)
}

// BuildEmailToken generates a cryptographically random one-time token, stores
// its SHA-256 hash, and returns the raw (unhashed) hex token.
//
// tokenContext identifies the purpose of the token (e.g. "login", "confirm").
func (r *Repository) BuildEmailToken(ctx context.Context, user *User, tokenContext string) (string, error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", fmt.Errorf("accounts.BuildEmailToken: generate random bytes: %w", err)
	}
	rawToken := hex.EncodeToString(rawBytes)

	hash := sha256.Sum256(rawBytes)

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("accounts.BuildEmailToken: generate id: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	_, err = q.CreateEmailToken(ctx, sqlc.CreateEmailTokenParams{
		ID:         id.String(),
		UserID:     user.ID,
		TokenHash:  hash[:],
		Context:    tokenContext,
		SentTo:     user.Email,
		InsertedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("accounts.BuildEmailToken: %w", err)
	}

	return rawToken, nil
}

// ConsumeEmailToken looks up the token by its hash and tokenContext, validates
// it has not expired (insertedAt + validity >= now), deletes it, and returns
// the associated User.
//
// Returns an error if the token is not found, already used, or expired.
func (r *Repository) ConsumeEmailToken(ctx context.Context, rawToken, tokenContext string, validity time.Duration) (*User, error) {
	rawBytes, err := hex.DecodeString(rawToken)
	if err != nil {
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: decode token: %w", err)
	}

	hash := sha256.Sum256(rawBytes)

	q := sqlc.New(r.db.Write)
	row, err := q.GetEmailTokenByHashAndContext(ctx, sqlc.GetEmailTokenByHashAndContextParams{
		TokenHash: hash[:],
		Context:   tokenContext,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: lookup: %w", err)
	}

	insertedAt, err := time.Parse(time.RFC3339Nano, row.InsertedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: parse inserted_at: %w", err)
	}

	if time.Since(insertedAt) > validity {
		// Delete the expired token so it cannot be used again.
		_ = q.DeleteEmailToken(ctx, row.ID)
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: token expired")
	}

	if err := q.DeleteEmailToken(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: delete token: %w", err)
	}

	user, err := r.GetUserByID(ctx, row.UserID)
	if err != nil {
		return nil, fmt.Errorf("accounts.ConsumeEmailToken: get user: %w", err)
	}

	return user, nil
}

// userFromRow maps a sqlc.User row to the domain User type.
func userFromRow(row sqlc.User) (*User, error) {
	insertedAt, err := time.Parse(time.RFC3339Nano, row.InsertedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts: parse inserted_at %q: %w", row.InsertedAt, err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts: parse updated_at %q: %w", row.UpdatedAt, err)
	}

	u := &User{
		ID:         row.ID,
		Email:      row.Email,
		InsertedAt: insertedAt,
		UpdatedAt:  updatedAt,
	}

	if row.HashedPassword != nil {
		u.HashedPassword = *row.HashedPassword
	}

	if row.ConfirmedAt != nil {
		t, err := time.Parse(time.RFC3339Nano, *row.ConfirmedAt)
		if err != nil {
			return nil, fmt.Errorf("accounts: parse confirmed_at %q: %w", *row.ConfirmedAt, err)
		}
		u.ConfirmedAt = &t
	}

	return u, nil
}
