-- Indexes for sorting the QSO index by a field other than date. Each sort
-- orders by whether the field is empty (empty values last), the field, then
-- start time and ID, newest first; a reversed sort turns only the field
-- around, so each field has an index for each direction. The expressions
-- must match the ORDER BY terms the logbook builds (internal/logbook/sort.go)
-- or SQLite will not use them.

CREATE INDEX qso_sort_call         ON qso ((call = ''), call,      start_utc DESC, id DESC);
CREATE INDEX qso_sort_call_reverse ON qso ((call = ''), call DESC, start_utc DESC, id DESC);

CREATE INDEX qso_sort_country         ON qso ((country = ''), country COLLATE NOCASE,      start_utc DESC, id DESC);
CREATE INDEX qso_sort_country_reverse ON qso ((country = ''), country COLLATE NOCASE DESC, start_utc DESC, id DESC);

CREATE INDEX qso_sort_state         ON qso ((state = ''), state COLLATE NOCASE,      start_utc DESC, id DESC);
CREATE INDEX qso_sort_state_reverse ON qso ((state = ''), state COLLATE NOCASE DESC, start_utc DESC, id DESC);

CREATE INDEX qso_sort_cont         ON qso ((cont = ''), cont,      start_utc DESC, id DESC);
CREATE INDEX qso_sort_cont_reverse ON qso ((cont = ''), cont DESC, start_utc DESC, id DESC);

-- A mode sorts by its submode when it has one, so FT4 sorts as FT4.
CREATE INDEX qso_sort_mode ON qso ((CASE WHEN submode <> '' THEN submode ELSE mode END = ''), (CASE WHEN submode <> '' THEN submode ELSE mode END), start_utc DESC, id DESC);
CREATE INDEX qso_sort_mode_reverse ON qso ((CASE WHEN submode <> '' THEN submode ELSE mode END = ''), (CASE WHEN submode <> '' THEN submode ELSE mode END) DESC, start_utc DESC, id DESC);

-- Bands sort by frequency: each band's position in the ADIF Band enumeration
-- (bands in internal/logbook/band.go), with any other band after them all.
CREATE INDEX qso_sort_band ON qso (
    (band = ''),
    (CASE band
        WHEN '2190m' THEN 0
        WHEN '630m' THEN 1
        WHEN '560m' THEN 2
        WHEN '160m' THEN 3
        WHEN '80m' THEN 4
        WHEN '60m' THEN 5
        WHEN '40m' THEN 6
        WHEN '30m' THEN 7
        WHEN '20m' THEN 8
        WHEN '17m' THEN 9
        WHEN '15m' THEN 10
        WHEN '12m' THEN 11
        WHEN '10m' THEN 12
        WHEN '8m' THEN 13
        WHEN '6m' THEN 14
        WHEN '5m' THEN 15
        WHEN '4m' THEN 16
        WHEN '2m' THEN 17
        WHEN '1.25m' THEN 18
        WHEN '70cm' THEN 19
        WHEN '33cm' THEN 20
        WHEN '23cm' THEN 21
        WHEN '13cm' THEN 22
        WHEN '9cm' THEN 23
        WHEN '6cm' THEN 24
        WHEN '3cm' THEN 25
        WHEN '1.25cm' THEN 26
        WHEN '6mm' THEN 27
        WHEN '4mm' THEN 28
        WHEN '2.5mm' THEN 29
        WHEN '2mm' THEN 30
        WHEN '1mm' THEN 31
        ELSE 32
    END),
    start_utc DESC,
    id DESC
);

CREATE INDEX qso_sort_band_reverse ON qso (
    (band = ''),
    (CASE band
        WHEN '2190m' THEN 0
        WHEN '630m' THEN 1
        WHEN '560m' THEN 2
        WHEN '160m' THEN 3
        WHEN '80m' THEN 4
        WHEN '60m' THEN 5
        WHEN '40m' THEN 6
        WHEN '30m' THEN 7
        WHEN '20m' THEN 8
        WHEN '17m' THEN 9
        WHEN '15m' THEN 10
        WHEN '12m' THEN 11
        WHEN '10m' THEN 12
        WHEN '8m' THEN 13
        WHEN '6m' THEN 14
        WHEN '5m' THEN 15
        WHEN '4m' THEN 16
        WHEN '2m' THEN 17
        WHEN '1.25m' THEN 18
        WHEN '70cm' THEN 19
        WHEN '33cm' THEN 20
        WHEN '23cm' THEN 21
        WHEN '13cm' THEN 22
        WHEN '9cm' THEN 23
        WHEN '6cm' THEN 24
        WHEN '3cm' THEN 25
        WHEN '1.25cm' THEN 26
        WHEN '6mm' THEN 27
        WHEN '4mm' THEN 28
        WHEN '2.5mm' THEN 29
        WHEN '2mm' THEN 30
        WHEN '1mm' THEN 31
        ELSE 32
    END) DESC,
    start_utc DESC,
    id DESC
);
