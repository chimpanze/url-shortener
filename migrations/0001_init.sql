CREATE TABLE links (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  slug        TEXT    NOT NULL UNIQUE,
  destination TEXT    NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_links_created_at ON links(created_at DESC);

CREATE TABLE clicks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  ts         INTEGER NOT NULL,
  referer    TEXT    NOT NULL DEFAULT '',
  user_agent TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_clicks_link_ts ON clicks(link_id, ts DESC);

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT    NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token       TEXT    PRIMARY KEY,
  csrf_token  TEXT    NOT NULL,
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL
);
