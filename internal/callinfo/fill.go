package callinfo

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"

	"jstlogger/internal/callsign"
	"jstlogger/internal/dxcc"
	"jstlogger/internal/logbook"
)

// DXCCLookup resolves callsigns to DXCC entities. *dxcc.Table satisfies
// it.
type DXCCLookup interface {
	// Lookup returns the DXCC entity for call, and whether call is in one.
	Lookup(call string) (dxcc.Entity, bool)
}

// Fill returns q with what is known about its call filled into the fields
// it leaves empty: the DXCC entity, continent and CQ and ITU zones, the
// name, QTH, grid, state, county and email from Details, and the country,
// ADIF's name for the QSO's DXCC entity code, whether that code was found
// here or came with q. A value q already has is kept, so a grid the station
// sent wins over the callbook's. A call without a base callsign is not
// looked up.
//
// The error reports a lookup that failed; the QSO returned is still q with
// whatever was found, ready to store.
func (l Looker) Fill(ctx context.Context, q logbook.QSO) (logbook.QSO, error) {
	call := strings.ToUpper(strings.TrimSpace(q.Call))
	if callsign.Base(call) == "" {
		return q, nil
	}
	if l.DXCC != nil {
		if ent, ok := l.DXCC.Lookup(call); ok {
			fillEntity(&q, ent)
		}
	}
	fillCountry(&q)
	d, err := l.Details(ctx, call)
	fillDetails(&q, d)
	if err != nil {
		return q, fmt.Errorf("look up %s: %w", call, err)
	}
	return q, nil
}

// fillEntity fills ent's DXCC code, continent and zones into the fields of
// q that are empty.
func fillEntity(q *logbook.QSO, ent dxcc.Entity) {
	fillNull(&q.DXCC, ent.DXCC)
	fillString(&q.Cont, ent.Continent)
	fillNull(&q.CQZ, ent.CQZone)
	fillNull(&q.ITUZ, ent.ITUZone)
}

// fillCountry fills ADIF's name for q's DXCC entity code into q's country
// when it is empty. A code of 0, which is no entity, or one ADIF does not
// list leaves it empty.
func fillCountry(q *logbook.QSO) {
	if !q.DXCC.Valid {
		return
	}
	if name, ok := dxcc.EntityName(q.DXCC.V); ok {
		fillString(&q.Country, name)
	}
}

// fillDetails fills d into the fields of q that are empty.
func fillDetails(q *logbook.QSO, d Details) {
	fillString(&q.Name, d.Name)
	fillString(&q.QTH, d.QTH)
	fillString(&q.Gridsquare, d.Grid)
	fillString(&q.State, d.State)
	fillString(&q.Cnty, d.Cnty)
	if d.Email != "" && q.Fields["EMAIL"] == "" {
		// Fields may be shared with the caller's QSO.
		q.Fields = maps.Clone(q.Fields)
		if q.Fields == nil {
			q.Fields = map[string]string{}
		}
		q.Fields["EMAIL"] = d.Email
	}
}

// fillString sets *dst to v when *dst is blank.
func fillString(dst *string, v string) {
	if strings.TrimSpace(*dst) == "" {
		*dst = v
	}
}

// fillNull sets *dst to v when *dst is null.
func fillNull(dst *sql.Null[int], v int) {
	if !dst.Valid {
		*dst = sql.Null[int]{V: v, Valid: true}
	}
}
