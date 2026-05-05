-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
  id              TEXT PRIMARY KEY,
  email           TEXT NOT NULL COLLATE NOCASE,
  hashed_password TEXT,
  confirmed_at    TEXT,
  inserted_at     TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX users_email_idx ON users(email COLLATE NOCASE);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE email_tokens (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  token_hash  BLOB NOT NULL,
  context     TEXT NOT NULL,
  sent_to     TEXT NOT NULL,
  inserted_at TEXT NOT NULL,
  UNIQUE(context, token_hash)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX email_tokens_user_idx ON email_tokens(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE teams (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL COLLATE NOCASE,
  global_admin INTEGER NOT NULL DEFAULT 0,
  inserted_at  TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  CHECK (name <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX teams_name_idx ON teams(name COLLATE NOCASE);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_team_roles (
  team_id     TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  user_id     TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  title       TEXT NOT NULL,
  inserted_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  UNIQUE(team_id, user_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX user_team_roles_user_idx ON user_team_roles(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE products (
  id          TEXT PRIMARY KEY,
  team_id     TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  name        TEXT NOT NULL COLLATE NOCASE,
  description TEXT,
  is_active   INTEGER NOT NULL DEFAULT 1,
  inserted_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  UNIQUE(team_id, name),
  CHECK (name <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE access_tokens (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  team_id       TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  name          TEXT NOT NULL,
  token_hash    TEXT NOT NULL,
  token_prefix  TEXT NOT NULL,
  scopes        TEXT NOT NULL,
  expires_at    TEXT,
  revoked_at    TEXT,
  last_used_at  TEXT,
  inserted_at   TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX access_tokens_hash_idx ON access_tokens(token_hash);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX access_tokens_prefix_idx ON access_tokens(token_prefix);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX access_tokens_user_idx ON access_tokens(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX access_tokens_team_idx ON access_tokens(team_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE implementations (
  id                       TEXT PRIMARY KEY,
  product_id               TEXT NOT NULL REFERENCES products ON DELETE CASCADE,
  team_id                  TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  parent_implementation_id TEXT REFERENCES implementations ON DELETE SET NULL,
  name                     TEXT NOT NULL,
  description              TEXT,
  is_active                INTEGER NOT NULL DEFAULT 1,
  inserted_at              TEXT NOT NULL,
  updated_at               TEXT NOT NULL,
  UNIQUE(product_id, name)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX implementations_team_idx ON implementations(team_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX implementations_parent_idx ON implementations(parent_implementation_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE branches (
  id                TEXT PRIMARY KEY,
  team_id           TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  repo_uri          TEXT NOT NULL,
  branch_name       TEXT NOT NULL,
  last_seen_commit  TEXT NOT NULL,
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  UNIQUE(team_id, repo_uri, branch_name)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX branches_repo_idx ON branches(repo_uri);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE tracked_branches (
  implementation_id TEXT NOT NULL REFERENCES implementations ON DELETE CASCADE,
  branch_id         TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  repo_uri          TEXT NOT NULL,
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  PRIMARY KEY (implementation_id, branch_id),
  UNIQUE(implementation_id, repo_uri)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX tracked_branches_branch_idx ON tracked_branches(branch_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE specs (
  id                  TEXT PRIMARY KEY,
  product_id          TEXT NOT NULL REFERENCES products ON DELETE CASCADE,
  branch_id           TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  path                TEXT,
  last_seen_commit    TEXT NOT NULL,
  parsed_at           TEXT NOT NULL,
  feature_name        TEXT NOT NULL,
  feature_description TEXT,
  feature_version     TEXT NOT NULL DEFAULT '1.0.0',
  raw_content         TEXT,
  requirements        TEXT NOT NULL DEFAULT '{}',
  inserted_at         TEXT NOT NULL,
  updated_at          TEXT NOT NULL,
  UNIQUE(branch_id, feature_name),
  CHECK (feature_name <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX specs_product_idx ON specs(product_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX specs_branch_idx ON specs(branch_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE feature_impl_states (
  id                TEXT PRIMARY KEY,
  implementation_id TEXT NOT NULL REFERENCES implementations ON DELETE CASCADE,
  feature_name      TEXT NOT NULL,
  states            TEXT NOT NULL DEFAULT '{}',
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  UNIQUE(implementation_id, feature_name),
  CHECK (feature_name <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX feature_impl_states_impl_idx ON feature_impl_states(implementation_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE feature_branch_refs (
  id           TEXT PRIMARY KEY,
  branch_id    TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  feature_name TEXT NOT NULL,
  refs         TEXT NOT NULL DEFAULT '{}',
  commit       TEXT NOT NULL,
  pushed_at    TEXT NOT NULL,
  inserted_at  TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE(branch_id, feature_name),
  CHECK (feature_name <> '')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX feature_branch_refs_branch_idx ON feature_branch_refs(branch_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feature_branch_refs;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS feature_impl_states;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS specs;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS tracked_branches;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS branches;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS implementations;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS access_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS products;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS user_team_roles;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS teams;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS email_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
