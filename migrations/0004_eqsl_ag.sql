-- The eQSL Authenticity Guaranteed (AG) member list: every callsign whose
-- eQSL account is AG, plus a single row recording the last successful
-- download of the list.

CREATE TABLE eqsl_ag (
    call TEXT NOT NULL PRIMARY KEY                 -- upper-case callsign as listed, e.g. W1AW or KH6/K1ABC
) WITHOUT ROWID;

-- Only written by a successful sync, so a missing row means "never synced".
CREATE TABLE eqsl_sync (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    etag          TEXT    NOT NULL DEFAULT '',     -- ETag response header, for If-None-Match
    last_modified TEXT    NOT NULL DEFAULT '',     -- Last-Modified response header, for If-Modified-Since
    synced_at     TEXT    NOT NULL                 -- RFC 3339, UTC
);
