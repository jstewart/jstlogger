package logbook

import (
	"database/sql/driver"
	"fmt"

	"modernc.org/sqlite"

	"jstlogger/internal/callsign"
)

// callBaseFunc is the SQL function that returns the base callsign of its
// argument, as callsign.Base does, or NULL for NULL.
const callBaseFunc = "callsign_base"

func init() {
	// The driver applies the function to every connection it opens from
	// now on, so it must be registered before Open.
	if err := sqlite.RegisterDeterministicScalarFunction(callBaseFunc, 1, sqlCallBase); err != nil {
		panic(fmt.Sprintf("logbook: register %s: %v", callBaseFunc, err))
	}
}

// sqlCallBase implements callBaseFunc.
func sqlCallBase(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	switch v := args[0].(type) {
	case nil:
		return nil, nil
	case string:
		return callsign.Base(v), nil
	case []byte:
		return callsign.Base(string(v)), nil
	default:
		return nil, fmt.Errorf("%s: unsupported argument type %T", callBaseFunc, v)
	}
}

// CallBaseIs matches QSOs whose call has the same base callsign as Call, as
// callsign.Base finds it, so CallBaseIs{"W1AW"} matches VE3/W1AW and
// W1AW/P. A Call without a base callsign, such as "", matches every QSO.
type CallBaseIs struct {
	// Call is the callsign whose base to match.
	Call string
}

// sql implements Expr. The LIKE narrows the QSOs to those whose call holds
// the base, so the base of only those is worked out.
func (c CallBaseIs) sql() (string, []any, error) {
	base := callsign.Base(c.Call)
	if base == "" {
		return sqlTrue, nil, nil
	}
	return `call LIKE ? ESCAPE '\' AND ` + callBaseFunc + `(call) = ?`,
		[]any{"%" + likeEscaper.Replace(base) + "%", base}, nil
}
