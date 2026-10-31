-- The ADIF OPERATOR field: the callsign of the person who operated our
-- station, which can differ from STATION_CALLSIGN (e.g. at a club station).

ALTER TABLE qso ADD COLUMN operator TEXT NOT NULL DEFAULT '';
