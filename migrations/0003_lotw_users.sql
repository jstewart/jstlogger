-- The LoTW user activity feed: every callsign that has uploaded to Logbook
-- of The World, with the time of its most recent upload, plus a single row
-- recording the last successful download of the feed.

CREATE TABLE lotw_user (
    call        TEXT NOT NULL PRIMARY KEY,         -- upper-case callsign as listed in the feed
    last_upload TEXT NOT NULL                      -- RFC 3339, UTC, e.g. 2026-10-28T14:42:17Z
) WITHOUT ROWID;

-- Only written by a successful sync, so a missing row means "never synced".
CREATE TABLE lotw_sync (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    etag          TEXT    NOT NULL DEFAULT '',     -- ETag response header, for If-None-Match
    last_modified TEXT    NOT NULL DEFAULT '',     -- Last-Modified response header, for If-Modified-Since
    synced_at     TEXT    NOT NULL                 -- RFC 3339, UTC
);
