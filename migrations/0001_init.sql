-- Initial logbook schema: QSOs with promoted ADIF columns, a bag for every
-- other ADIF field, per-service confirmations and user tags.

CREATE TABLE qso (
    id               INTEGER PRIMARY KEY,
    uuid             TEXT    NOT NULL UNIQUE,
    start_utc        TEXT    NOT NULL,            -- RFC 3339, UTC, e.g. 2026-10-28T14:42:17Z
    end_utc          TEXT,
    call             TEXT    NOT NULL,
    station_callsign TEXT    NOT NULL DEFAULT '',
    freq_hz          INTEGER,
    freq_rx_hz       INTEGER,
    band             TEXT    NOT NULL DEFAULT '',
    mode             TEXT    NOT NULL DEFAULT '',
    submode          TEXT    NOT NULL DEFAULT '',
    rst_sent         TEXT    NOT NULL DEFAULT '',
    rst_rcvd         TEXT    NOT NULL DEFAULT '',
    tx_pwr           REAL,                        -- watts
    name             TEXT    NOT NULL DEFAULT '',
    qth              TEXT    NOT NULL DEFAULT '',
    gridsquare       TEXT    NOT NULL DEFAULT '',
    my_gridsquare    TEXT    NOT NULL DEFAULT '',
    dxcc             INTEGER,
    country          TEXT    NOT NULL DEFAULT '',
    cont             TEXT    NOT NULL DEFAULT '',
    cqz              INTEGER,
    ituz             INTEGER,
    itu_region       INTEGER CHECK (itu_region IN (1, 2, 3)),
    distance_km      REAL,
    state            TEXT    NOT NULL DEFAULT '',
    cnty             TEXT    NOT NULL DEFAULT '',
    comment          TEXT    NOT NULL DEFAULT '',
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL
);

CREATE INDEX qso_by_start       ON qso (start_utc DESC, id DESC);
CREATE INDEX qso_by_call        ON qso (call);
CREATE INDEX qso_by_dxcc        ON qso (dxcc);
CREATE INDEX qso_by_band        ON qso (band);
CREATE INDEX qso_by_mode        ON qso (mode);
CREATE INDEX qso_by_cont        ON qso (cont);
CREATE INDEX qso_by_itu_region  ON qso (itu_region);
CREATE INDEX qso_by_distance    ON qso (distance_km);
CREATE INDEX qso_worked_before  ON qso (call, band, mode);

-- ADIF fields without a column of their own. A field lives in exactly one
-- place: promoted fields never appear here.
CREATE TABLE qso_adif_field (
    qso_id INTEGER NOT NULL REFERENCES qso (id) ON DELETE CASCADE,
    name   TEXT    NOT NULL,                      -- upper-case ADIF field name
    value  TEXT    NOT NULL,
    PRIMARY KEY (qso_id, name)
);

CREATE TABLE qso_confirmation (
    qso_id    INTEGER NOT NULL REFERENCES qso (id) ON DELETE CASCADE,
    service   TEXT    NOT NULL,                   -- 'lotw' | 'eqsl' | 'card' | …
    sent      TEXT    NOT NULL DEFAULT '',        -- ADIF QSL_SENT enum
    sent_date TEXT,
    rcvd      TEXT    NOT NULL DEFAULT '',        -- ADIF QSL_RCVD enum
    rcvd_date TEXT,
    PRIMARY KEY (qso_id, service)
);

CREATE INDEX qso_confirmation_by_rcvd ON qso_confirmation (rcvd, service, qso_id);

CREATE TABLE tag (
    id   INTEGER PRIMARY KEY,
    name TEXT    NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE qso_tag (
    qso_id INTEGER NOT NULL REFERENCES qso (id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES tag (id) ON DELETE CASCADE,
    PRIMARY KEY (qso_id, tag_id)
);

CREATE INDEX qso_tag_by_tag ON qso_tag (tag_id, qso_id);
