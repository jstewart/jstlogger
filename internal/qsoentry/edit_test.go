package qsoentry

import (
	"database/sql"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// serviceQRZ is a confirmation service the form does not show.
const serviceQRZ = "qrz"

// microwaveForm returns fullForm on 23 cm with a 70 cm receive freq, a
// frequency at or above 1000 MHz, an FM mode and a fractional power.
func microwaveForm() Form {
	f := fullForm()
	f[Mode], f[Submode], f[RSTSent], f[RSTRcvd] = "FM", "", "59", "57"
	f[Freq], f[Band] = "1296.2 MHz", ""
	f[FreqRX], f[BandRX] = "435.5", ""
	f[TxPwr] = "5.5"
	return f
}

// storedQSO returns the QSO f builds with what the form does not show added
// to it, as a QSO read back from the logbook would have: an ID, UUID,
// country, distance and timestamps, fields of its own, received QSLs with
// dates, a confirmation from a service the form has no status for and
// another tag.
func storedQSO(t *testing.T, f Form) logbook.QSO {
	t.Helper()
	q, err := Build(f, testNow)
	require.NoError(t, err)

	q.ID, q.UUID = 7, "6f1c2d3e-0000-4000-8000-000000000007"
	q.Country = "United States"
	q.DistanceKm = sql.Null[float64]{V: 123.4, Valid: true}
	q.ITURegion = sql.Null[int]{V: 2, Valid: true}
	q.CreatedAt = time.Date(2026, 11, 9, 14, 11, 0, 0, time.UTC)
	q.UpdatedAt = time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)
	q.Fields["CONTEST_ID"] = "ARRL-DX-CW"
	q.Fields["APP_X"] = "kept"
	q.Tags = []string{"contest", "pota"}

	sent := sql.Null[time.Time]{V: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), Valid: true}
	rcvd := sql.Null[time.Time]{V: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), Valid: true}
	for i, c := range q.Confirmations {
		c.SentDate = sent
		if c.Service != logbook.ServiceClubLog {
			c.Rcvd, c.RcvdDate = "Y", rcvd
		}
		q.Confirmations[i] = c
	}
	q.Confirmations = append(q.Confirmations, logbook.Confirmation{
		Service: serviceQRZ, Sent: "Y", SentDate: sent, Rcvd: "Y", RcvdDate: rcvd,
	})
	return q
}

// cloneQSO returns a deep copy of q, so a test can change it without
// changing q.
func cloneQSO(q logbook.QSO) logbook.QSO {
	q.Fields = maps.Clone(q.Fields)
	q.Confirmations = slices.Clone(q.Confirmations)
	q.Tags = slices.Clone(q.Tags)
	return q
}

// confirmation returns q's confirmation with service, failing the test when
// there is none.
func confirmation(t *testing.T, q logbook.QSO, service string) *logbook.Confirmation {
	t.Helper()
	i := slices.IndexFunc(q.Confirmations, func(c logbook.Confirmation) bool { return c.Service == service })
	require.GreaterOrEqual(t, i, 0, "no %s confirmation in %v", service, q.Confirmations)
	return &q.Confirmations[i]
}

// openStore opens a new SQLite logbook in a temp dir.
func openStore(t *testing.T) *logbook.SQLiteStore {
	t.Helper()
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return store
}

func TestLoad(t *testing.T) {
	f := Load(storedQSO(t, fullForm()))

	for k := range FieldCount {
		assert.NotEmpty(t, f[k], "%s is filled in", k)
	}
	assert.Equal(t, Form{
		Date: "2026-11-09", TimeOn: "14:05:00", DateOff: "2026-11-09", TimeOff: "14:10:00",
		Call: "K1ABC", Mode: "MFSK", Submode: "FT4",
		Freq: "14.08", Band: "20m", FreqRX: "14.081", BandRX: "20m",
		RSTSent: "-10", RSTRcvd: "-12",
		Name: "Ann", QTH: "Boston", Gridsquare: "FN42", Comment: "hi", Tags: "contest, pota",

		StationName: "home", StationCallsign: "W1AW", Operator: "K1XX", MyRig: "IC-7300",
		TxPwr: "100", MyGridsquare: "FN31PR", MyPOTARef: "US-0001,US-0002",
		MySOTARef: "W1/HA-001", MyIOTA: "NA-001", MyWWFFRef: "KFF-1234",
		MyState: "CT", MyCity: "Newington",

		DXCC: "291", Cont: "NA", Region: "NONE", CQZ: "5", ITUZ: "8",
		State: "MA", Cnty: "MA,Suffolk", IOTA: "NA-046", SOTARef: "W1/MA-001",
		WWFFRef: "KFF-0001", POTARef: "US-1234,US-5678", Sig: "WAB", SigInfo: "TQ38",
		DarcDok: "B36", Email: "ann@example.com", PropMode: "SAT", AntPath: "S",

		SatName: "AO-91", SatMode: "U/V",

		Notes: "first line\nsecond line",

		QSLSent: "Q", QSLSentVia: "B", QSLVia: "via K1MGR", QSLMsg: "TNX QSO",
		LoTWSent: "Y", EQSLSent: "R", ClubLogSent: "N",
	}, f)
}

func TestLoadFreqs(t *testing.T) {
	tests := []struct {
		name string
		hz   int64
		want string
	}{
		{"HF", 14_074_000, "14.074"},
		{"whole MHz", 50_000_000, "50"},
		{"below 1000 MHz", 999_999_999, "999.999999"},
		{"1000 MHz", 1_000_000_000, "1000 MHz"},
		{"microwave", 10_368_100_000, "10368.1 MHz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := logbook.QSO{
				FreqHz:   sql.Null[int64]{V: tt.hz, Valid: true},
				FreqRxHz: sql.Null[int64]{V: tt.hz, Valid: true},
			}

			f := Load(q)

			assert.Equal(t, tt.want, f[Freq])
			assert.Equal(t, tt.want, f[FreqRX])
			hz, err := ParseFreq(f[Freq])
			require.NoError(t, err)
			assert.Equal(t, tt.hz, hz, "the freq survives ParseFreq")
		})
	}
}

func TestLoadEmpty(t *testing.T) {
	q := logbook.QSO{StartUTC: time.Date(2026, 11, 9, 14, 5, 7, 0, time.UTC), Call: "K1ABC"}

	f := Load(q)

	var want Form
	want[Date], want[TimeOn], want[Call] = "2026-11-09", "14:05:07", "K1ABC"
	assert.Equal(t, want, f, "what the QSO lacks stays empty, DXCC and zones included")
}

func TestLoadUTC(t *testing.T) {
	east := time.FixedZone("UTC+10", 10*60*60)
	q := logbook.QSO{
		StartUTC: time.Date(2026, 11, 10, 8, 0, 0, 0, east),
		EndUTC:   sql.Null[time.Time]{V: time.Date(2026, 11, 10, 8, 30, 0, 0, east), Valid: true},
	}

	f := Load(q)

	assert.Equal(t, [4]string{"2026-11-09", "22:00:00", "2026-11-09", "22:30:00"},
		[4]string{f[Date], f[TimeOn], f[DateOff], f[TimeOff]}, "dates and times are UTC")
}

func TestEditUnchanged(t *testing.T) {
	for _, form := range []func() Form{fullForm, microwaveForm} {
		orig := storedQSO(t, form())

		got, err := Edit(Load(orig), orig, testNow)

		require.NoError(t, err)
		assert.Equal(t, orig, got, "loading a QSO and saving it unchanged changes nothing")
	}
}

func TestEditTabs(t *testing.T) {
	tests := []struct {
		name  string
		field Field
		value string
		// want applies the edit to the original QSO.
		want func(q *logbook.QSO)
	}{
		{"QSO call", Call, "k1xyz", func(q *logbook.QSO) { q.Call = "K1XYZ" }},
		{"QSO comment cleared", Comment, "", func(q *logbook.QSO) { q.Comment = "" }},
		{"QSO tags", Tags, "pota, sota", func(q *logbook.QSO) { q.Tags = []string{"pota", "sota"} }},
		{"Station rig", MyRig, "FTdx10", func(q *logbook.QSO) { q.Fields["MY_RIG"] = "FTdx10" }},
		{"Station power", TxPwr, "50", func(q *logbook.QSO) { q.TxPwr = sql.Null[float64]{V: 50, Valid: true} }},
		{"Station name cleared", StationName, "", func(q *logbook.QSO) { delete(q.Fields, logbook.StationField) }},
		{"General DXCC", DXCC, "1", func(q *logbook.QSO) { q.DXCC = sql.Null[int]{V: 1, Valid: true} }},
		{"General email", Email, "ann@example.org", func(q *logbook.QSO) { q.Fields["EMAIL"] = "ann@example.org" }},
		{"General CQ zone cleared", CQZ, "", func(q *logbook.QSO) { q.CQZ = sql.Null[int]{} }},
		{"Satellite name", SatName, "so-50", func(q *logbook.QSO) { q.Fields["SAT_NAME"] = "SO-50" }},
		{"Notes", Notes, "new note", func(q *logbook.QSO) { q.Fields["NOTES"] = "new note" }},
		{"QSL via cleared", QSLVia, "", func(q *logbook.QSO) { delete(q.Fields, "QSL_VIA") }},
		{"QSL LoTW sent", LoTWSent, "n", func(q *logbook.QSO) {
			c := &q.Confirmations[slices.IndexFunc(q.Confirmations, func(c logbook.Confirmation) bool {
				return c.Service == logbook.ServiceLoTW
			})]
			c.Sent = "N"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := storedQSO(t, fullForm())
			want := cloneQSO(orig)
			tt.want(&want)
			f := Load(orig)
			f[tt.field] = tt.value

			got, err := Edit(f, orig, testNow)

			require.NoError(t, err)
			assert.Equal(t, want, got, "only the edited field changes")
		})
	}
}

func TestEditFreqAndBand(t *testing.T) {
	orig := storedQSO(t, fullForm())
	f := Load(orig)
	f[Freq] = "7.074"

	_, err := Edit(f, orig, testNow)

	errs := fieldErrors(t, err)
	assert.Equal(t, []Field{Band}, errs.Fields(), "Build's errors are returned as they are")

	f[Band] = ""
	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.Equal(t, sql.Null[int64]{V: 7_074_000, Valid: true}, got.FreqHz)
	assert.Equal(t, "40m", got.Band, "a cleared band comes from the new freq")
}

func TestEditDoesNotMutateOrig(t *testing.T) {
	orig := storedQSO(t, fullForm())
	before := cloneQSO(orig)
	f := Load(orig)
	f[Call], f[MyRig], f[Notes], f[QSLVia] = "K1XYZ", "FTdx10", "", ""
	f[QSLSent], f[LoTWSent], f[ClubLogSent] = "", "N", ""
	f[PropMode] = "ES"

	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.Equal(t, before, orig, "Edit leaves orig alone")
	got.Fields["APP_X"] = "changed"
	got.Confirmations[0].Rcvd = "N"
	assert.Equal(t, before, orig, "the edited QSO shares no maps or slices with orig")
}

func TestEditKeepsSatelliteWhenNotSAT(t *testing.T) {
	orig := storedQSO(t, fullForm())
	f := Load(orig)
	f[PropMode] = "es"
	f[SatName] = "SO-50"

	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.Equal(t, "ES", got.Fields["PROP_MODE"])
	assert.Equal(t, "AO-91", got.Fields["SAT_NAME"], "the stored satellite is kept when PROP_MODE is not SAT")
	assert.Equal(t, "U/V", got.Fields["SAT_MODE"])

	f[PropMode] = ""
	got, err = Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.NotContains(t, got.Fields, "PROP_MODE", "a cleared propagation mode is removed")
	assert.Equal(t, "AO-91", got.Fields["SAT_NAME"])
}

func TestEditSatelliteCleared(t *testing.T) {
	orig := storedQSO(t, fullForm())
	f := Load(orig)
	f[SatName], f[SatMode] = "", ""

	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.NotContains(t, got.Fields, "SAT_NAME", "the satellite is the form's when PROP_MODE is SAT")
	assert.NotContains(t, got.Fields, "SAT_MODE")
}

func TestEditConfirmations(t *testing.T) {
	orig := storedQSO(t, fullForm())
	// No LoTW confirmation yet.
	orig.Confirmations = slices.DeleteFunc(orig.Confirmations, func(c logbook.Confirmation) bool {
		return c.Service == logbook.ServiceLoTW
	})
	// A Club Log confirmation that holds nothing but its sent status.
	confirmation(t, orig, logbook.ServiceClubLog).SentDate = sql.Null[time.Time]{}
	f := Load(orig)
	require.Empty(t, f[LoTWSent])
	f[QSLSent] = ""
	f[LoTWSent] = "q"
	f[ClubLogSent] = ""

	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	services := make([]string, 0, len(got.Confirmations))
	for _, c := range got.Confirmations {
		services = append(services, c.Service)
	}
	assert.Equal(t, []string{logbook.ServiceCard, logbook.ServiceEQSL, serviceQRZ, logbook.ServiceLoTW}, services,
		"the original order is kept, new confirmations follow and empty ones are dropped")

	card := confirmation(t, got, logbook.ServiceCard)
	origCard := confirmation(t, orig, logbook.ServiceCard)
	assert.Empty(t, card.Sent, "the card's sent status is cleared")
	assert.Equal(t, origCard.Rcvd, card.Rcvd, "the card's received status is kept")
	assert.Equal(t, origCard.SentDate, card.SentDate)
	assert.Equal(t, origCard.RcvdDate, card.RcvdDate)

	assert.Equal(t, logbook.Confirmation{Service: logbook.ServiceLoTW, Sent: "Q"}, *confirmation(t, got, logbook.ServiceLoTW))
	assert.Equal(t, *confirmation(t, orig, serviceQRZ), *confirmation(t, got, serviceQRZ), "other services are kept")
}

func TestEditNoFields(t *testing.T) {
	orig, err := Build(validForm(), testNow)
	require.NoError(t, err)
	orig.ID = 1
	orig.Fields = map[string]string{"NOTES": "only a note"}
	f := Load(orig)
	f[Notes] = ""

	got, err := Edit(f, orig, testNow)

	require.NoError(t, err)
	assert.Nil(t, got.Fields, "no fields left is nil, as Build has it")
	assert.Nil(t, got.Confirmations)
}

func TestEditInvalid(t *testing.T) {
	orig := storedQSO(t, fullForm())
	f := Load(orig)
	f[Call], f[Email] = "", "nope"

	got, err := Edit(f, orig, testNow)

	errs := fieldErrors(t, err)
	assert.Equal(t, []Field{Call, Email}, errs.Fields())
	assert.Equal(t, logbook.QSO{}, got)
}

func TestEditStoreRoundTrip(t *testing.T) {
	for name, form := range map[string]func() Form{"HF": fullForm, "microwave": microwaveForm} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			store := openStore(t)
			q := storedQSO(t, form())
			q.ID, q.UUID = 0, ""
			added, err := store.Add(ctx, q)
			require.NoError(t, err)

			before, err := store.Get(ctx, added.ID)
			require.NoError(t, err)
			edited, err := Edit(Load(before), before, testNow)
			require.NoError(t, err)
			require.NoError(t, store.Update(ctx, edited))
			after, err := store.Get(ctx, added.ID)
			require.NoError(t, err)

			assert.Equal(t, "United States", after.Country)
			assert.Equal(t, sql.Null[float64]{V: 123.4, Valid: true}, after.DistanceKm)
			assert.Len(t, after.Confirmations, 5)
			after.UpdatedAt = before.UpdatedAt
			assert.Equal(t, before, after, "saving a loaded QSO unchanged changes nothing in the logbook")
		})
	}
}

func TestEditStoreRoundTripWithChanges(t *testing.T) {
	ctx := t.Context()
	store := openStore(t)
	q := storedQSO(t, fullForm())
	q.ID, q.UUID = 0, ""
	added, err := store.Add(ctx, q)
	require.NoError(t, err)
	before, err := store.Get(ctx, added.ID)
	require.NoError(t, err)

	f := Load(before)
	f[Call], f[MyRig], f[DXCC], f[SatName], f[Notes], f[LoTWSent] = "K1XYZ", "FTdx10", "1", "SO-50", "edited", "N"
	edited, err := Edit(f, before, testNow)
	require.NoError(t, err)
	require.NoError(t, store.Update(ctx, edited))
	after, err := store.Get(ctx, added.ID)
	require.NoError(t, err)

	want := cloneQSO(before)
	want.Call = "K1XYZ"
	want.DXCC = sql.Null[int]{V: 1, Valid: true}
	want.Fields["MY_RIG"] = "FTdx10"
	want.Fields["SAT_NAME"] = "SO-50"
	want.Fields["NOTES"] = "edited"
	confirmation(t, want, logbook.ServiceLoTW).Sent = "N"
	want.UpdatedAt = after.UpdatedAt
	assert.Equal(t, want, after, "one edit per tab is stored and nothing else changes")
}

func TestEditStoreRecalculatesDerivedFields(t *testing.T) {
	ctx := t.Context()
	store := openStore(t)
	q := storedQSO(t, fullForm())
	q.ID, q.UUID = 0, ""
	added, err := store.Add(ctx, q)
	require.NoError(t, err)
	before, err := store.Get(ctx, added.ID)
	require.NoError(t, err)
	require.Equal(t, sql.Null[float64]{V: 123.4, Valid: true}, before.DistanceKm)
	require.Equal(t, sql.Null[int]{V: 2, Valid: true}, before.ITURegion)

	// A grid in Europe with no ITU zone moves the QSO to region 1, and the
	// new grid replaces the stored distance with one worked out from it.
	f := Load(before)
	f[Gridsquare], f[ITUZ] = "JO62", ""
	edited, err := Edit(f, before, testNow)
	require.NoError(t, err)
	require.NoError(t, store.Update(ctx, edited))
	after, err := store.Get(ctx, added.ID)
	require.NoError(t, err)

	fresh, err := Build(f, testNow)
	require.NoError(t, err)
	want, err := store.Add(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, sql.Null[int]{V: 1, Valid: true}, after.ITURegion)
	require.True(t, want.DistanceKm.Valid)
	assert.Equal(t, want.DistanceKm, after.DistanceKm, "the distance is worked out from the new grid")
}
