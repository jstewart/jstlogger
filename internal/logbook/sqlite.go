package logbook

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"jstlogger/migrations"
)

// Options configures a SQLiteStore.
type Options struct {
	// StationGrid is the configured station Maidenhead locator. It is used
	// to compute a QSO's distance when the QSO has no MyGridsquare.
	StationGrid string
}

// SQLiteStore is a Store backed by a SQLite database file.
type SQLiteStore struct {
	db   *sql.DB
	opts Options
	// now returns the current time; tests replace it.
	now func() time.Time
}

var _ Store = (*SQLiteStore)(nil)

// dsnPragmas are applied by the driver to every new connection. _txlock
// makes transactions take the write lock when they begin: in WAL mode a
// deferred transaction that reads and then writes fails at once with
// SQLITE_BUSY if another connection wrote in between, and busy_timeout does
// not retry that.
const dsnPragmas = "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"

// Open opens, creating if needed, the SQLite logbook at path and migrates it
// to the latest schema.
func Open(ctx context.Context, path string, opts Options) (*SQLiteStore, error) {
	dsn, err := sqliteDSN(path)
	if err != nil {
		return nil, fmt.Errorf("logbook: open %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("logbook: open %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("logbook: open %s: %w", path, err), db.Close())
	}
	if err := migrate(ctx, db, migrations.FS); err != nil {
		return nil, errors.Join(fmt.Errorf("logbook: open %s: %w", path, err), db.Close())
	}
	return &SQLiteStore{db: db, opts: opts, now: time.Now}, nil
}

// sqliteDSN builds a file: URI for path with the store's connection pragmas.
func sqliteDSN(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		// Windows drive paths ("C:/x") need a leading slash in a file URI.
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed, RawQuery: dsnPragmas}
	return u.String(), nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("logbook: close: %w", err)
	}
	return nil
}

// qsoColumns lists the qso table's columns in the order scanQSO reads them.
const qsoColumns = `id, uuid, start_utc, end_utc, call, station_callsign,
	operator, freq_hz, freq_rx_hz, band, mode, submode, rst_sent, rst_rcvd, tx_pwr,
	name, qth, gridsquare, my_gridsquare, dxcc, country, cont, cqz, ituz,
	itu_region, distance_km, state, cnty, comment, created_at, updated_at`

// Add stores a new QSO and returns it as Get would.
func (s *SQLiteStore) Add(ctx context.Context, q QSO) (QSO, error) {
	q, err := prepare(q, s.opts.StationGrid)
	if err != nil {
		return QSO{}, fmt.Errorf("logbook: add qso: %w", err)
	}
	if q.UUID == "" {
		q.UUID = newUUID()
	}
	now := s.now()
	q.CreatedAt, q.UpdatedAt = now, now

	var id int64
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = insertQSO(ctx, tx, q)
		return err
	})
	if err != nil {
		return QSO{}, fmt.Errorf("logbook: add qso: %w", err)
	}

	stored, err := s.Get(ctx, id)
	if err != nil {
		return QSO{}, fmt.Errorf("logbook: add qso: %w", err)
	}
	return stored, nil
}

// Get returns the QSO with the given ID, or an error wrapping ErrNotFound.
func (s *SQLiteStore) Get(ctx context.Context, id int64) (QSO, error) {
	q, err := s.get(ctx, id)
	if err != nil {
		return QSO{}, fmt.Errorf("logbook: get qso %d: %w", id, err)
	}
	return q, nil
}

func (s *SQLiteStore) get(ctx context.Context, id int64) (QSO, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+qsoColumns+" FROM qso WHERE id = ?", id)
	q, err := scanQSO(row)
	if errors.Is(err, sql.ErrNoRows) {
		return QSO{}, ErrNotFound
	}
	if err != nil {
		return QSO{}, err
	}
	qs := []QSO{q}
	if err := s.loadChildren(ctx, qs); err != nil {
		return QSO{}, err
	}
	return qs[0], nil
}

// Update replaces the stored QSO with q.ID, keeping its UUID and creation
// time, or returns an error wrapping ErrNotFound.
func (s *SQLiteStore) Update(ctx context.Context, q QSO) error {
	id := q.ID
	q, err := prepare(q, s.opts.StationGrid)
	if err != nil {
		return fmt.Errorf("logbook: update qso %d: %w", id, err)
	}
	q.UpdatedAt = s.now()

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var oldGrid, oldMyGrid string
		err := tx.QueryRowContext(ctx, "SELECT gridsquare, my_gridsquare FROM qso WHERE id = ?", q.ID).
			Scan(&oldGrid, &oldMyGrid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read qso: %w", err)
		}
		q.DistanceKm = s.updatedDistance(q, oldGrid, oldMyGrid)

		res, err := tx.ExecContext(ctx, `UPDATE qso SET
			start_utc = ?, end_utc = ?, call = ?, station_callsign = ?, operator = ?,
			freq_hz = ?, freq_rx_hz = ?, band = ?, mode = ?, submode = ?,
			rst_sent = ?, rst_rcvd = ?, tx_pwr = ?, name = ?, qth = ?,
			gridsquare = ?, my_gridsquare = ?, dxcc = ?, country = ?, cont = ?,
			cqz = ?, ituz = ?, itu_region = ?, distance_km = ?, state = ?,
			cnty = ?, comment = ?, updated_at = ?
			WHERE id = ?`,
			formatTime(q.StartUTC), nullTime(q.EndUTC), q.Call, q.StationCallsign, q.Operator,
			nullArg(q.FreqHz), nullArg(q.FreqRxHz), q.Band, q.Mode, q.Submode,
			q.RSTSent, q.RSTRcvd, nullArg(q.TxPwr), q.Name, q.QTH,
			q.Gridsquare, q.MyGridsquare, nullArg(q.DXCC), q.Country, q.Cont,
			nullArg(q.CQZ), nullArg(q.ITUZ), nullArg(q.ITURegion), nullArg(q.DistanceKm), q.State,
			q.Cnty, q.Comment, formatTime(q.UpdatedAt),
			q.ID)
		if err != nil {
			return fmt.Errorf("update qso: %w", err)
		}
		if err := requireOneRow(res); err != nil {
			return err
		}
		for _, table := range []string{"qso_adif_field", "qso_confirmation", "qso_tag"} {
			// table is one of the constants above, never input.
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE qso_id = ?", q.ID); err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		if err := insertChildren(ctx, tx, q.ID, q); err != nil {
			return err
		}
		return deleteUnusedTags(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("logbook: update qso %d: %w", q.ID, err)
	}
	return nil
}

// updatedDistance returns the distance to store when q replaces a QSO whose
// stored grids were oldGrid and oldMyGrid. prepare has already filled in
// q.DistanceKm if it was missing. If neither grid changed, q's distance is
// kept. If a grid changed, a distance calculated from the new grids replaces
// it; when none can be calculated, a distance the old grids produced is stale
// and is cleared, while one no grids could produce (an imported distance) is
// kept.
func (s *SQLiteStore) updatedDistance(q QSO, oldGrid, oldMyGrid string) sql.Null[float64] {
	if upper(oldGrid) == q.Gridsquare && upper(oldMyGrid) == q.MyGridsquare {
		return q.DistanceKm
	}
	if km, ok := gridDistance(q.Gridsquare, q.MyGridsquare, s.opts.StationGrid); ok {
		return sql.Null[float64]{V: km, Valid: true}
	}
	if _, wasCalculated := gridDistance(oldGrid, oldMyGrid, s.opts.StationGrid); wasCalculated {
		return sql.Null[float64]{}
	}
	return q.DistanceKm
}

// Delete removes the QSO with the given ID and, through foreign keys, its
// fields, confirmations and tag links. It returns an error wrapping
// ErrNotFound if there is no such QSO.
func (s *SQLiteStore) Delete(ctx context.Context, id int64) error {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM qso WHERE id = ?", id)
		if err != nil {
			return err
		}
		if err := requireOneRow(res); err != nil {
			return err
		}
		return deleteUnusedTags(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("logbook: delete qso %d: %w", id, err)
	}
	return nil
}

// inTx runs fn in a transaction, committing if it succeeds and rolling back
// otherwise.
func (s *SQLiteStore) inTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, rollback(tx))
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// requireOneRow returns ErrNotFound if res affected no rows.
func requireOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// insertQSO inserts q, which prepare has normalized and which has its UUID
// and timestamps set, with its fields, confirmations and tags, and returns
// its new ID.
func insertQSO(ctx context.Context, tx *sql.Tx, q QSO) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO qso (
		uuid, start_utc, end_utc, call, station_callsign, operator,
		freq_hz, freq_rx_hz, band, mode, submode, rst_sent, rst_rcvd, tx_pwr,
		name, qth, gridsquare, my_gridsquare, dxcc, country, cont, cqz, ituz,
		itu_region, distance_km, state, cnty, comment, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.UUID, formatTime(q.StartUTC), nullTime(q.EndUTC), q.Call, q.StationCallsign, q.Operator,
		nullArg(q.FreqHz), nullArg(q.FreqRxHz), q.Band, q.Mode, q.Submode, q.RSTSent, q.RSTRcvd, nullArg(q.TxPwr),
		q.Name, q.QTH, q.Gridsquare, q.MyGridsquare, nullArg(q.DXCC), q.Country, q.Cont, nullArg(q.CQZ), nullArg(q.ITUZ),
		nullArg(q.ITURegion), nullArg(q.DistanceKm), q.State, q.Cnty, q.Comment,
		formatTime(q.CreatedAt), formatTime(q.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("insert qso: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert qso: %w", err)
	}
	if err := insertChildren(ctx, tx, id, q); err != nil {
		return 0, err
	}
	return id, nil
}

// insertChildren inserts q's ADIF fields, confirmations and tags for the QSO
// with the given ID.
func insertChildren(ctx context.Context, tx *sql.Tx, id int64, q QSO) error {
	for _, name := range slices.Sorted(maps.Keys(q.Fields)) {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO qso_adif_field (qso_id, name, value) VALUES (?, ?, ?)",
			id, name, q.Fields[name]); err != nil {
			return fmt.Errorf("insert field %s: %w", name, err)
		}
	}
	for _, c := range q.Confirmations {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO qso_confirmation (qso_id, service, sent, sent_date, rcvd, rcvd_date)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, c.Service, c.Sent, nullTime(c.SentDate), c.Rcvd, nullTime(c.RcvdDate)); err != nil {
			return fmt.Errorf("insert confirmation %s: %w", c.Service, err)
		}
	}
	return attachTags(ctx, tx, id, q.Tags)
}

// attachTags links the QSO with the given ID to each tag, creating tags that
// do not exist yet. Tag names match ignoring case; an existing tag keeps its
// spelling.
func attachTags(ctx context.Context, tx *sql.Tx, qsoID int64, tags []string) error {
	for _, name := range tags {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO tag (name) VALUES (?)", name); err != nil {
			return fmt.Errorf("create tag %q: %w", name, err)
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM tag WHERE name = ?", name).Scan(&tagID); err != nil {
			return fmt.Errorf("look up tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO qso_tag (qso_id, tag_id) VALUES (?, ?)", qsoID, tagID); err != nil {
			return fmt.Errorf("attach tag %q: %w", name, err)
		}
	}
	return nil
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanQSO reads the qsoColumns of one row.
func scanQSO(r scanner) (QSO, error) {
	var (
		q                       QSO
		start, created, updated string
		end                     sql.Null[string]
	)
	err := r.Scan(&q.ID, &q.UUID, &start, &end, &q.Call, &q.StationCallsign,
		&q.Operator, &q.FreqHz, &q.FreqRxHz, &q.Band, &q.Mode, &q.Submode, &q.RSTSent, &q.RSTRcvd, &q.TxPwr,
		&q.Name, &q.QTH, &q.Gridsquare, &q.MyGridsquare, &q.DXCC, &q.Country, &q.Cont, &q.CQZ, &q.ITUZ,
		&q.ITURegion, &q.DistanceKm, &q.State, &q.Cnty, &q.Comment, &created, &updated)
	if err != nil {
		return QSO{}, fmt.Errorf("scan qso: %w", err)
	}
	if q.StartUTC, err = parseTime(start); err != nil {
		return QSO{}, fmt.Errorf("scan qso %d start_utc: %w", q.ID, err)
	}
	if q.EndUTC, err = parseNullTime(end); err != nil {
		return QSO{}, fmt.Errorf("scan qso %d end_utc: %w", q.ID, err)
	}
	if q.CreatedAt, err = parseTime(created); err != nil {
		return QSO{}, fmt.Errorf("scan qso %d created_at: %w", q.ID, err)
	}
	if q.UpdatedAt, err = parseTime(updated); err != nil {
		return QSO{}, fmt.Errorf("scan qso %d updated_at: %w", q.ID, err)
	}
	return q, nil
}

// loadChildren fills in the ADIF fields, confirmations and tags of each QSO
// in qs, which must have distinct IDs. It runs one query per child table
// whatever the number of QSOs.
func (s *SQLiteStore) loadChildren(ctx context.Context, qs []QSO) error {
	if len(qs) == 0 {
		return nil
	}
	ids := make([]any, len(qs))
	index := make(map[int64]int, len(qs))
	for i, q := range qs {
		ids[i] = q.ID
		index[q.ID] = i
	}
	in := "(" + placeholders(len(ids)) + ")"
	if err := s.loadFields(ctx, in, ids, func(id int64, name, value string) {
		q := &qs[index[id]]
		if q.Fields == nil {
			q.Fields = make(map[string]string)
		}
		q.Fields[name] = value
	}); err != nil {
		return err
	}
	if err := s.loadConfirmations(ctx, in, ids, func(id int64, c Confirmation) {
		q := &qs[index[id]]
		q.Confirmations = append(q.Confirmations, c)
	}); err != nil {
		return err
	}
	return s.loadTags(ctx, in, ids, func(id int64, name string) {
		q := &qs[index[id]]
		q.Tags = append(q.Tags, name)
	})
}

// loadFields calls add for each ADIF field of the QSOs whose IDs are given;
// in is the matching "(?, …)" placeholder list.
func (s *SQLiteStore) loadFields(ctx context.Context, in string, ids []any, add func(id int64, name, value string)) (err error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT qso_id, name, value FROM qso_adif_field WHERE qso_id IN "+in, ids...)
	if err != nil {
		return fmt.Errorf("query fields: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		var (
			id          int64
			name, value string
		)
		if err := rows.Scan(&id, &name, &value); err != nil {
			return fmt.Errorf("scan field: %w", err)
		}
		add(id, name, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query fields: %w", err)
	}
	return nil
}

// loadConfirmations calls add for each confirmation of the QSOs whose IDs
// are given, in service order per QSO; in is the matching "(?, …)"
// placeholder list.
func (s *SQLiteStore) loadConfirmations(ctx context.Context, in string, ids []any, add func(id int64, c Confirmation)) (err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT qso_id, service, sent, sent_date, rcvd, rcvd_date FROM qso_confirmation
		WHERE qso_id IN `+in+` ORDER BY qso_id, service`, ids...)
	if err != nil {
		return fmt.Errorf("query confirmations: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		var (
			id             int64
			c              Confirmation
			sent, received sql.Null[string]
		)
		if err := rows.Scan(&id, &c.Service, &c.Sent, &sent, &c.Rcvd, &received); err != nil {
			return fmt.Errorf("scan confirmation: %w", err)
		}
		if c.SentDate, err = parseNullTime(sent); err != nil {
			return fmt.Errorf("scan confirmation %s sent_date: %w", c.Service, err)
		}
		if c.RcvdDate, err = parseNullTime(received); err != nil {
			return fmt.Errorf("scan confirmation %s rcvd_date: %w", c.Service, err)
		}
		add(id, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query confirmations: %w", err)
	}
	return nil
}

// loadTags calls add for each tag name of the QSOs whose IDs are given, in
// name order per QSO; in is the matching "(?, …)" placeholder list.
func (s *SQLiteStore) loadTags(ctx context.Context, in string, ids []any, add func(id int64, name string)) (err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT qt.qso_id, t.name FROM tag t JOIN qso_tag qt ON qt.tag_id = t.id
		WHERE qt.qso_id IN `+in+` ORDER BY qt.qso_id, t.name`, ids...)
	if err != nil {
		return fmt.Errorf("query tags: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("scan tag: %w", err)
		}
		add(id, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query tags: %w", err)
	}
	return nil
}

// placeholders returns n comma-separated "?" placeholders; n must be > 0.
func placeholders(n int) string {
	return strings.Repeat("?, ", n-1) + "?"
}

// closeRows closes rows, wrapping any error.
func closeRows(rows *sql.Rows) error {
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close rows: %w", err)
	}
	return nil
}

// formatTime encodes t as RFC 3339 UTC text with second precision, so that
// text order equals time order.
func formatTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// parseTime decodes a time written by formatTime.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time: %w", err)
	}
	return t.UTC(), nil
}

// nullTime encodes an optional time as formatTime text or NULL.
func nullTime(t sql.Null[time.Time]) any {
	if !t.Valid {
		return nil
	}
	return formatTime(t.V)
}

// parseNullTime decodes an optional time written by nullTime.
func parseNullTime(s sql.Null[string]) (sql.Null[time.Time], error) {
	if !s.Valid {
		return sql.Null[time.Time]{}, nil
	}
	t, err := parseTime(s.V)
	if err != nil {
		return sql.Null[time.Time]{}, err
	}
	return sql.Null[time.Time]{V: t, Valid: true}, nil
}

// nullArg returns n's value as a query argument, or nil for NULL.
func nullArg[T any](n sql.Null[T]) any {
	if !n.Valid {
		return nil
	}
	return n.V
}

// newUUID returns a random (version 4) UUID string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error.
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
