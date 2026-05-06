// Package migrate provides a one-shot Postgres → SQLite migration used to
// cut over from the Phoenix monolith to the Go rewrite.
//
// Usage:
//
//	err := migrate.PostgresToSQLite(ctx, pgURL, "/data/acai.db", true, os.Stdout)
package migrate

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// PostgresToSQLite imports a Postgres database into a fresh SQLite file at
// outPath. It runs migrations on the SQLite side, then streams rows from
// Postgres in FK order and translates types. If verify is true, it runs
// row-count and sample-join checks after the import.
//
// Type-translation rules:
//   - UUID / citext / text → TEXT (hyphens preserved)
//   - timestamptz → TEXT RFC3339Nano
//   - jsonb → TEXT (JSON string)
//   - bytea → BLOB (raw bytes)
//   - bool → INTEGER (1/0)
//   - integer / bigint → INTEGER
func PostgresToSQLite(ctx context.Context, pgURL, outPath string, verify bool, log io.Writer) error {
	pgPool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer pgPool.Close()

	if _, err2 := pgPool.Exec(ctx, "SELECT 1"); err2 != nil {
		return fmt.Errorf("postgres ping: %w", err2)
	}
	_, _ = fmt.Fprintln(log, "connected to Postgres")

	db, err := store.Open(outPath)
	if err != nil {
		return fmt.Errorf("sqlite open: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := store.RunMigrations(ctx, db); err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}
	_, _ = fmt.Fprintln(log, "SQLite schema ready")

	// Disable FK enforcement for bulk load; re-enable at end.
	if _, err := db.Write.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable fk: %w", err)
	}
	defer func() {
		_, _ = db.Write.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	}()

	_, _ = fmt.Fprintln(log, "starting import...")

	counts := map[string]int{}

	// Tables in FK order: parent tables first.
	type tableSpec struct {
		pgName     string // table name in Postgres
		sqliteName string // table name in SQLite (may differ)
		fn         func(context.Context, *pgxpool.Pool, *sql.DB, io.Writer) (int, error)
	}

	tables := []tableSpec{
		{"users", "users", importUsers},
		{"users_tokens", "email_tokens", importEmailTokens},
		{"teams", "teams", importTeams},
		{"user_team_roles", "user_team_roles", importUserTeamRoles},
		{"products", "products", importProducts},
		{"access_tokens", "access_tokens", importAccessTokens},
		{"branches", "branches", importBranches},
		{"implementations", "implementations", importImplementations},
		{"tracked_branches", "tracked_branches", importTrackedBranches},
		{"specs", "specs", importSpecs},
		{"feature_impl_states", "feature_impl_states", importFeatureImplStates},
		{"feature_branch_refs", "feature_branch_refs", importFeatureBranchRefs},
	}

	for _, t := range tables {
		n, err := t.fn(ctx, pgPool, db.Write, log)
		if err != nil {
			return fmt.Errorf("import %s: %w", t.pgName, err)
		}
		counts[t.sqliteName] = n
		_, _ = fmt.Fprintf(log, "  %-30s → %d rows\n", t.pgName, n)
	}

	if verify {
		return verifyImport(ctx, db.Read, counts, log)
	}

	_, _ = fmt.Fprintln(log, "import complete.")
	return nil
}

// --- per-table importers ---------------------------------------------------

func importUsers(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, email, hashed_password, confirmed_at, inserted_at, updated_at
		FROM users
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO users (id, email, hashed_password, confirmed_at, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id             string
			email          string
			hashedPassword *string
			confirmedAt    *time.Time
			insertedAt     time.Time
			updatedAt      time.Time
		)
		if err := rows.Scan(&id, &email, &hashedPassword, &confirmedAt, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id,
			email,
			hashedPassword,
			timeToText(confirmedAt),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

// importEmailTokens imports users_tokens from Postgres but filters out rows
// with context='session' (those are handled by cookie-based sessions in the Go
// rewrite and do not exist in the SQLite schema).
func importEmailTokens(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, user_id, token, context, sent_to, inserted_at
		FROM users_tokens
		WHERE context != 'session'
		ORDER BY inserted_at
	`)
	if err != nil {
		// Table may not exist in older schemas; treat as empty.
		if isTableNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO email_tokens (id, user_id, token_hash, context, sent_to, inserted_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id         string
			userID     string
			token      []byte
			tokenCtx   string
			sentTo     string
			insertedAt time.Time
		)
		if err := rows.Scan(&id, &userID, &token, &tokenCtx, &sentTo, &insertedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		// token is raw bytes in Postgres (bytea); store as BLOB in SQLite.
		_, err := stmt.ExecContext(ctx,
			id,
			userID,
			token,
			tokenCtx,
			sentTo,
			insertedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importTeams(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, name, global_admin, inserted_at, updated_at
		FROM teams
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO teams (id, name, global_admin, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id          string
			name        string
			globalAdmin bool
			insertedAt  time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&id, &name, &globalAdmin, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, name, boolToInt(globalAdmin),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importUserTeamRoles(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT team_id, user_id, title, inserted_at, updated_at
		FROM user_team_roles
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO user_team_roles (team_id, user_id, title, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			teamID     string
			userID     string
			title      string
			insertedAt time.Time
			updatedAt  time.Time
		)
		if err := rows.Scan(&teamID, &userID, &title, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			teamID, userID, title,
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importProducts(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, team_id, name, description, is_active, inserted_at, updated_at
		FROM products
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO products (id, team_id, name, description, is_active, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id          string
			teamID      string
			name        string
			description *string
			isActive    bool
			insertedAt  time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&id, &teamID, &name, &description, &isActive, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, teamID, name, description, boolToInt(isActive),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importAccessTokens(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, user_id, team_id, name, token_hash, token_prefix, scopes,
		       expires_at, revoked_at, last_used_at, inserted_at, updated_at
		FROM access_tokens
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO access_tokens
		  (id, user_id, team_id, name, token_hash, token_prefix, scopes,
		   expires_at, revoked_at, last_used_at, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id          string
			userID      string
			teamID      string
			name        string
			tokenHash   []byte
			tokenPrefix string
			scopes      string
			expiresAt   *time.Time
			revokedAt   *time.Time
			lastUsedAt  *time.Time
			insertedAt  time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(
			&id, &userID, &teamID, &name, &tokenHash, &tokenPrefix, &scopes,
			&expiresAt, &revokedAt, &lastUsedAt, &insertedAt, &updatedAt,
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		// token_hash in the Go schema is TEXT (hex-encoded); in Phoenix it may be
		// binary bytea. Hex-encode if the value looks like raw bytes (non-printable).
		tokenHashStr := bytesToHashText(tokenHash)
		_, err := stmt.ExecContext(ctx,
			id, userID, teamID, name, tokenHashStr, tokenPrefix, scopes,
			timeToText(expiresAt), timeToText(revokedAt), timeToText(lastUsedAt),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importBranches(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, team_id, repo_uri, branch_name, last_seen_commit, inserted_at, updated_at
		FROM branches
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO branches (id, team_id, repo_uri, branch_name, last_seen_commit, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id             string
			teamID         string
			repoURI        string
			branchName     string
			lastSeenCommit string
			insertedAt     time.Time
			updatedAt      time.Time
		)
		if err := rows.Scan(&id, &teamID, &repoURI, &branchName, &lastSeenCommit, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, teamID, repoURI, branchName, lastSeenCommit,
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importImplementations(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, product_id, team_id, parent_implementation_id,
		       name, description, is_active, inserted_at, updated_at
		FROM implementations
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO implementations
		  (id, product_id, team_id, parent_implementation_id,
		   name, description, is_active, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id                     string
			productID              string
			teamID                 string
			parentImplementationID *string
			name                   string
			description            *string
			isActive               bool
			insertedAt             time.Time
			updatedAt              time.Time
		)
		if err := rows.Scan(
			&id, &productID, &teamID, &parentImplementationID,
			&name, &description, &isActive, &insertedAt, &updatedAt,
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, productID, teamID, parentImplementationID,
			name, description, boolToInt(isActive),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importTrackedBranches(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT implementation_id, branch_id, repo_uri, inserted_at, updated_at
		FROM tracked_branches
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tracked_branches
		  (implementation_id, branch_id, repo_uri, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			implementationID string
			branchID         string
			repoURI          string
			insertedAt       time.Time
			updatedAt        time.Time
		)
		if err := rows.Scan(&implementationID, &branchID, &repoURI, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			implementationID, branchID, repoURI,
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importSpecs(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, product_id, branch_id, path, last_seen_commit, parsed_at,
		       feature_name, feature_description, feature_version,
		       raw_content, requirements, inserted_at, updated_at
		FROM specs
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO specs
		  (id, product_id, branch_id, path, last_seen_commit, parsed_at,
		   feature_name, feature_description, feature_version,
		   raw_content, requirements, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id                 string
			productID          string
			branchID           string
			path               *string
			lastSeenCommit     string
			parsedAt           time.Time
			featureName        string
			featureDescription *string
			featureVersion     string
			rawContent         *string
			requirementsRaw    []byte // jsonb
			insertedAt         time.Time
			updatedAt          time.Time
		)
		if err := rows.Scan(
			&id, &productID, &branchID, &path, &lastSeenCommit, &parsedAt,
			&featureName, &featureDescription, &featureVersion,
			&rawContent, &requirementsRaw, &insertedAt, &updatedAt,
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		requirements := jsonbToText(requirementsRaw)
		_, err := stmt.ExecContext(ctx,
			id, productID, branchID, path, lastSeenCommit,
			parsedAt.UTC().Format(time.RFC3339Nano),
			featureName, featureDescription, featureVersion,
			rawContent, requirements,
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importFeatureImplStates(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, implementation_id, feature_name, states, inserted_at, updated_at
		FROM feature_impl_states
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO feature_impl_states
		  (id, implementation_id, feature_name, states, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id               string
			implementationID string
			featureName      string
			statesRaw        []byte // jsonb
			insertedAt       time.Time
			updatedAt        time.Time
		)
		if err := rows.Scan(&id, &implementationID, &featureName, &statesRaw, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, implementationID, featureName, jsonbToText(statesRaw),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

func importFeatureBranchRefs(ctx context.Context, pg *pgxpool.Pool, db *sql.DB, _ io.Writer) (int, error) {
	rows, err := pg.Query(ctx, `
		SELECT id, branch_id, feature_name, refs, "commit", pushed_at, inserted_at, updated_at
		FROM feature_branch_refs
		ORDER BY inserted_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO feature_branch_refs
		  (id, branch_id, feature_name, refs, "commit", pushed_at, inserted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for rows.Next() {
		var (
			id          string
			branchID    string
			featureName string
			refsRaw     []byte // jsonb
			commit      string
			pushedAt    time.Time
			insertedAt  time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&id, &branchID, &featureName, &refsRaw, &commit, &pushedAt, &insertedAt, &updatedAt); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		_, err := stmt.ExecContext(ctx,
			id, branchID, featureName, jsonbToText(refsRaw), commit,
			pushedAt.UTC().Format(time.RFC3339Nano),
			insertedAt.UTC().Format(time.RFC3339Nano),
			updatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return n, tx.Commit()
}

// --- verification -----------------------------------------------------------

func verifyImport(ctx context.Context, db *sql.DB, pgCounts map[string]int, log io.Writer) error {
	_, _ = fmt.Fprintln(log, "verifying row counts...")
	tables := []string{
		"users", "email_tokens", "teams", "user_team_roles",
		"products", "access_tokens", "branches", "implementations",
		"tracked_branches", "specs", "feature_impl_states", "feature_branch_refs",
	}

	for _, t := range tables {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", t, err)
		}
		expected := pgCounts[t]
		if t == "email_tokens" {
			// The pg count is under "email_tokens" key after filtering.
			expected = pgCounts["email_tokens"]
		}
		if n != expected {
			return fmt.Errorf("row count mismatch for %s: sqlite=%d pg=%d", t, n, expected)
		}
		_, _ = fmt.Fprintf(log, "  %-30s OK (%d rows)\n", t, n)
	}

	// Sample join: tracked_branches → implementations, branches
	_, _ = fmt.Fprintln(log, "verifying sample joins...")
	var dangling int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tracked_branches tb
		WHERE NOT EXISTS (SELECT 1 FROM implementations i WHERE i.id = tb.implementation_id)
		   OR NOT EXISTS (SELECT 1 FROM branches b WHERE b.id = tb.branch_id)
	`).Scan(&dangling); err != nil {
		return fmt.Errorf("tracked_branches join check: %w", err)
	}
	if dangling > 0 {
		return fmt.Errorf("tracked_branches: %d rows with dangling FK references", dangling)
	}
	_, _ = fmt.Fprintln(log, "  tracked_branches FK integrity: OK")

	_, _ = fmt.Fprintln(log, "verification complete.")
	return nil
}

// --- helpers ----------------------------------------------------------------

func timeToText(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// jsonbToText marshals a jsonb value (returned as []byte by pgx) back to a
// canonical JSON string. Falls back to "{}" on error.
func jsonbToText(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	// pgx returns jsonb as raw JSON bytes; just convert to string.
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "{}"
	}
	// Validate that it is valid JSON; reformat to compact form.
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "{}"
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// bytesToHashText converts a bytea token hash to a hex string suitable for
// the TEXT token_hash column in SQLite. If the value already looks like a
// hex string (all printable ASCII) it is returned as-is.
func bytesToHashText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// If every byte is in the printable ASCII range, assume it's already text.
	allPrintable := true
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			allPrintable = false
			break
		}
	}
	if allPrintable {
		return string(b)
	}
	return hex.EncodeToString(b)
}

// isTableNotExist returns true when the pgx error indicates the table does not
// exist (Postgres error code 42P01 — undefined_table).
func isTableNotExist(err error) bool {
	if err == nil {
		return false
	}
	// Check for the Postgres error code directly via the error message, or via
	// the pgconn.PgError type if available in the error chain.
	return strings.Contains(err.Error(), "does not exist")
}
