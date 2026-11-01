package logbook

import (
	"strings"
)

// Expr is a boolean condition on QSOs that the store compiles to SQL. Only
// package logbook can implement it. A nil Expr matches every QSO.
type Expr interface {
	// sql returns the condition as a boolean SQL expression over the qso
	// table and its arguments. Values only ever travel as arguments.
	sql() (string, []any, error)
}

// Compile-time checks that the expression types implement Expr.
var (
	_ Expr = All{}
	_ Expr = And(nil)
	_ Expr = Or(nil)
	_ Expr = Not{}
	_ Expr = CallContains{}
	_ Expr = ModeIs{}
	_ Expr = Filter{}
)

const (
	// sqlTrue is the SQL expression that matches every QSO.
	sqlTrue = "1"
	// sqlFalse is the SQL expression that matches no QSO.
	sqlFalse = "0"
)

// exprSQL compiles e, treating nil as All, so callers never call a method on
// a nil interface.
func exprSQL(e Expr) (string, []any, error) {
	if e == nil {
		return sqlTrue, nil, nil
	}
	return e.sql()
}

// exprConds returns e compiled as a list of WHERE conditions for where:
// empty when e matches everything, so plain queries stay unconstrained. The
// condition is parenthesized so that AND-ing it with others cannot change
// its meaning.
func exprConds(e Expr) ([]string, []any, error) {
	clause, args, err := exprSQL(e)
	if err != nil {
		return nil, nil, err
	}
	if clause == sqlTrue {
		return nil, args, nil
	}
	return []string{"(" + clause + ")"}, args, nil
}

// All matches every QSO.
type All struct{}

// sql implements Expr.
func (All) sql() (string, []any, error) { return sqlTrue, nil, nil }

// And matches QSOs that match all of its children. An empty And matches
// every QSO.
type And []Expr

// sql implements Expr.
func (a And) sql() (string, []any, error) {
	return join(a, " AND ", sqlTrue)
}

// Or matches QSOs that match any of its children. An empty Or matches no
// QSO.
type Or []Expr

// sql implements Expr.
func (o Or) sql() (string, []any, error) {
	return join(o, " OR ", sqlFalse)
}

// join compiles each of children, wraps it in parentheses and joins them
// with op, or returns empty when there are no children.
func join(children []Expr, op, empty string) (string, []any, error) {
	if len(children) == 0 {
		return empty, nil, nil
	}
	parts := make([]string, 0, len(children))
	var args []any
	for _, c := range children {
		clause, a, err := exprSQL(c)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, "("+clause+")")
		args = append(args, a...)
	}
	return strings.Join(parts, op), args, nil
}

// Not matches QSOs that do not match X. A nil X is All, so Not{} matches no
// QSO.
type Not struct {
	// X is the negated expression.
	X Expr
}

// sql implements Expr.
func (n Not) sql() (string, []any, error) {
	clause, args, err := exprSQL(n.X)
	if err != nil {
		return "", nil, err
	}
	return "NOT (" + clause + ")", args, nil
}

// CallContains matches QSOs whose call contains Call, ignoring case and
// surrounding space. An empty Call matches every QSO.
type CallContains struct {
	// Call is the substring to look for.
	Call string
}

// likeEscaper escapes the LIKE wildcards and the escape character itself.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// sql implements Expr. Calls are stored upper-cased, so the argument is
// upper-cased to match.
func (c CallContains) sql() (string, []any, error) {
	call := upper(c.Call)
	if call == "" {
		return sqlTrue, nil, nil
	}
	return `call LIKE ? ESCAPE '\'`, []any{"%" + likeEscaper.Replace(call) + "%"}, nil
}

// ModeIs matches QSOs whose mode or submode is Mode, ignoring case and
// surrounding space, so ModeIs{"FT4"} matches MODE=MFSK SUBMODE=FT4. An
// empty Mode matches every QSO.
type ModeIs struct {
	// Mode is the mode or submode to match.
	Mode string
}

// sql implements Expr. Mode and submode are normalized on write, so the
// argument is normalized the same way.
func (m ModeIs) sql() (string, []any, error) {
	mode := upper(m.Mode)
	if mode == "" {
		return sqlTrue, nil, nil
	}
	return "(mode = ? OR submode = ?)", []any{mode, mode}, nil
}

// sql implements Expr. The zero Filter matches every QSO.
func (f Filter) sql() (string, []any, error) {
	clause, args, err := filterSQL(f)
	if err != nil {
		return "", nil, err
	}
	if clause == "" {
		return sqlTrue, nil, nil
	}
	return clause, args, nil
}
