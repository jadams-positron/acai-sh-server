-- +goose Up
-- +goose StatementBegin
CREATE TABLE events (
  id            TEXT PRIMARY KEY,
  team_id       TEXT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
  product_id    TEXT REFERENCES products (id) ON DELETE CASCADE,
  impl_id       TEXT REFERENCES implementations (id) ON DELETE CASCADE,
  feature_name  TEXT,
  actor_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
  kind          TEXT NOT NULL,
  payload       TEXT NOT NULL,
  inserted_at   TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_team_idx
  ON events (team_id, inserted_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_product_idx
  ON events (product_id, inserted_at DESC) WHERE product_id IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_impl_idx
  ON events (impl_id, inserted_at DESC) WHERE impl_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS events_impl_idx;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS events_product_idx;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS events_team_idx;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS events;
-- +goose StatementEnd
