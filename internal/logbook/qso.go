package logbook

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned (possibly wrapped) when a QSO does not exist.
var ErrNotFound = errors.New("logbook: qso not found")

// Confirmation services. A service is a free-form lower-case string, so a
// new service needs no schema change; these are the well-known ones.
const (
	// ServiceLoTW is ARRL Logbook of The World.
	ServiceLoTW = "lotw"
	// ServiceEQSL is eQSL.cc.
	ServiceEQSL = "eqsl"
	// ServiceCard is a paper QSL card (the ADIF QSL_* fields).
	ServiceCard = "card"
)

// QSO is one logged contact.
//
// Optional strings use "" for absent. Optional numeric and time fields use
// sql.Null, because zero is a real value for several of them (a distance of
// 0 km, DXCC entity 0 meaning "none").
type QSO struct {
	// ID is the database row ID; 0 for a QSO that has not been stored.
	ID int64
	// UUID is a stable identifier used for dedupe and sync.
	UUID string
	// StartUTC is when the QSO started (ADIF QSO_DATE + TIME_ON). Required.
	StartUTC time.Time
	// EndUTC is when the QSO ended (ADIF QSO_DATE_OFF + TIME_OFF).
	EndUTC sql.Null[time.Time]
	// Call is the contacted station's callsign (ADIF CALL).
	Call string
	// StationCallsign is the callsign we used (ADIF STATION_CALLSIGN).
	StationCallsign string
	// Operator is the callsign of the person who operated our station (ADIF OPERATOR).
	Operator string
	// FreqHz is the transmit frequency in Hz (ADIF FREQ).
	FreqHz sql.Null[int64]
	// FreqRxHz is the receive frequency in Hz when split (ADIF FREQ_RX).
	FreqRxHz sql.Null[int64]
	// Band is the ADIF band enumeration value, e.g. "20m" (ADIF BAND).
	Band string
	// Mode is the ADIF mode (ADIF MODE).
	Mode string
	// Submode is the ADIF submode, e.g. "FT4" under "MFSK" (ADIF SUBMODE).
	Submode string
	// RSTSent is the signal report we sent (ADIF RST_SENT).
	RSTSent string
	// RSTRcvd is the signal report we received (ADIF RST_RCVD).
	RSTRcvd string
	// TxPwr is our transmit power in watts (ADIF TX_PWR).
	TxPwr sql.Null[float64]
	// Name is the contacted operator's name (ADIF NAME).
	Name string
	// QTH is the contacted station's location (ADIF QTH).
	QTH string
	// Gridsquare is the contacted station's Maidenhead locator (ADIF GRIDSQUARE).
	Gridsquare string
	// MyGridsquare is our Maidenhead locator (ADIF MY_GRIDSQUARE).
	MyGridsquare string
	// DXCC is the contacted station's DXCC entity number (ADIF DXCC).
	DXCC sql.Null[int]
	// Country is the DXCC entity name (ADIF COUNTRY).
	Country string
	// Cont is the continent code, e.g. "NA" (ADIF CONT).
	Cont string
	// CQZ is the CQ zone (ADIF CQZ).
	CQZ sql.Null[int]
	// ITUZ is the ITU zone (ADIF ITUZ).
	ITUZ sql.Null[int]
	// ITURegion is the ITU region, 1, 2 or 3. It is not an ADIF field; it is
	// derived on every write from ITUZ or Gridsquare, and any value set by
	// the caller is ignored.
	ITURegion sql.Null[int]
	// DistanceKm is the great-circle distance to the contacted station in km
	// (ADIF DISTANCE). A supplied distance is kept as is. When there is none,
	// it is calculated from the grids on write when possible, rounded to
	// 0.1 km. Update recalculates it when Gridsquare or MyGridsquare changes.
	DistanceKm sql.Null[float64]
	// State is the contacted station's state or province (ADIF STATE).
	State string
	// Cnty is the contacted station's county (ADIF CNTY).
	Cnty string
	// Comment is a free-text comment (ADIF COMMENT).
	Comment string
	// Fields holds every ADIF field that is not promoted to a QSO field,
	// keyed by upper-case ADIF field name.
	Fields map[string]string
	// Confirmations holds the QSL status for each confirmation service.
	Confirmations []Confirmation
	// Tags are the operator's own labels for the QSO.
	Tags []string
	// CreatedAt is when the QSO was first stored.
	CreatedAt time.Time
	// UpdatedAt is when the QSO was last stored.
	UpdatedAt time.Time
}

// Confirmation is the QSL status of a QSO with one confirmation service.
type Confirmation struct {
	// Service names the confirmation service, e.g. ServiceLoTW.
	Service string
	// Sent is the ADIF QSL_SENT enumeration (Y/N/R/Q/I), "" if unknown.
	Sent string
	// SentDate is when the QSL was sent.
	SentDate sql.Null[time.Time]
	// Rcvd is the ADIF QSL_RCVD enumeration (Y/N/R/I), "" if unknown.
	Rcvd string
	// RcvdDate is when the QSL was received.
	RcvdDate sql.Null[time.Time]
}

// Tag is an operator-defined label that can be applied to QSOs.
type Tag struct {
	// ID is the database row ID.
	ID int64
	// Name is the tag's name; names are unique ignoring case.
	Name string
}

// Confirmed selects QSOs by confirmation status in a Filter.
type Confirmed int

const (
	// ConfirmedAny does not filter by confirmation status.
	ConfirmedAny Confirmed = iota
	// ConfirmedYes selects QSOs with a received confirmation.
	ConfirmedYes
	// ConfirmedNo selects QSOs without a received confirmation.
	ConfirmedNo
)

// Filter narrows the QSOs returned by Store.List and counted by Store.Count.
//
// Values within one slice are OR'd and the dimensions are AND'd. An empty
// slice or zero value places no constraint.
type Filter struct {
	// From is the inclusive lower bound on StartUTC; zero is unbounded.
	From time.Time
	// To is the exclusive upper bound on StartUTC; zero is unbounded.
	To time.Time
	// DXCC selects QSOs with any of these DXCC entity numbers.
	DXCC []int
	// ITURegions selects QSOs in any of these ITU regions.
	ITURegions []int
	// Modes selects QSOs with any of these modes.
	Modes []string
	// Bands selects QSOs on any of these bands.
	Bands []string
	// Countries selects QSOs with any of these country names.
	Countries []string
	// Continents selects QSOs on any of these continents.
	Continents []string
	// Tags selects QSOs that carry ALL of these tags.
	Tags []string
	// MinDistanceKm is the inclusive minimum distance. QSOs without a
	// distance are excluded when set.
	MinDistanceKm sql.Null[float64]
	// MaxDistanceKm is the inclusive maximum distance. QSOs without a
	// distance are excluded when set.
	MaxDistanceKm sql.Null[float64]
	// Confirmed selects by confirmation status.
	Confirmed Confirmed
	// ConfirmedService restricts Confirmed to one service; "" means any.
	ConfirmedService string
}

// Cursor marks a position in the newest-first QSO ordering (StartUTC
// descending, then ID descending) for keyset paging.
type Cursor struct {
	// StartUTC is the StartUTC of the last QSO on the previous page.
	StartUTC time.Time
	// ID is the ID of the last QSO on the previous page.
	ID int64
}

// IsZero reports whether c is the zero Cursor, which starts from the newest
// QSO.
func (c Cursor) IsZero() bool {
	return c.StartUTC.IsZero() && c.ID == 0
}

// PageRequest asks for one page of QSOs.
type PageRequest struct {
	// After is the position to continue from; the zero Cursor starts from
	// the newest QSO.
	After Cursor
	// Limit is the maximum number of QSOs to return.
	Limit int
}

// Page is one page of QSOs, newest first.
type Page struct {
	// QSOs are the QSOs on this page.
	QSOs []QSO
	// Next is the cursor to pass as PageRequest.After for the next page.
	Next Cursor
	// HasMore reports whether more QSOs follow this page.
	HasMore bool
}
