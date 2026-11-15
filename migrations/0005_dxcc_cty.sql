-- The DXCC country file: the cty.csv from the last successful download of
-- AD1C's Big CTY, kept whole and parsed when the logbook opens, with the
-- metadata of that download. It replaces the copy built into the program.

-- Only written by a successful sync, so a missing row means "never synced".
CREATE TABLE dxcc_cty (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    cty           BLOB    NOT NULL,                -- cty.csv, as extracted from the download
    etag          TEXT    NOT NULL DEFAULT '',     -- ETag response header, for If-None-Match
    last_modified TEXT    NOT NULL DEFAULT '',     -- Last-Modified response header, for If-Modified-Since
    synced_at     TEXT    NOT NULL                 -- RFC 3339, UTC
);
