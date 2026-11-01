package pattern

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"jstlogger/internal/logbook"
)

// ErrSyntax is wrapped by every error Parse returns for a malformed pattern.
var ErrSyntax = errors.New("syntax error")

// Parse parses a mutt-style pattern into a logbook expression. See the
// package documentation for the syntax. A single term is returned as is;
// runs of ANDed or ORed terms become a flat logbook.And or logbook.Or, and
// parentheses only group. Errors wrap ErrSyntax.
func Parse(s string) (logbook.Expr, error) {
	p := parser{src: []rune(s)}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, p.emptyOperand()
	}
	if p.peek() == ')' {
		return nil, p.errorf(p.pos, "unbalanced )")
	}
	return e, nil
}

// eof is returned by peek at the end of the input.
const eof rune = -1

// parser holds the state of a single Parse call. It scans runes directly,
// so positions are rune offsets and error columns count characters.
type parser struct {
	src []rune
	pos int
}

// errorf builds an error wrapping ErrSyntax that reports the 1-based
// column of the rune at offset pos.
func (p *parser) errorf(pos int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("pattern: %s at column %d: %w", msg, pos+1, ErrSyntax)
}

// skipSpace advances past whitespace.
func (p *parser) skipSpace() {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
}

// peek skips whitespace and returns the next rune without consuming it, or
// eof at the end of the input.
func (p *parser) peek() rune {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return eof
	}
	return p.src[p.pos]
}

// emptyOperand explains why no operand starts at the current position,
// where the grammar requires one.
func (p *parser) emptyOperand() error {
	switch p.peek() {
	case eof:
		return p.errorf(p.pos, "empty pattern")
	case ')':
		return p.errorf(p.pos, "unbalanced )")
	default: // '|', the only other rune that ends an AND run.
		return p.errorf(p.pos, "dangling |")
	}
}

// parseExpr parses andExpr ('|' andExpr)*. It returns nil without an error
// when no operand starts here, so callers can describe what is missing.
func (p *parser) parseExpr() (logbook.Expr, error) {
	first, err := p.parseAnd()
	if err != nil || first == nil {
		return nil, err
	}
	terms := []logbook.Expr{first}
	for p.peek() == '|' {
		bar := p.pos
		p.pos++
		next, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, p.errorf(bar, "dangling |")
		}
		terms = append(terms, next)
	}
	if len(terms) == 1 {
		return first, nil
	}
	return logbook.Or(terms), nil
}

// parseAnd parses a run of one or more juxtaposed unaries, ending at the end
// of the input, '|' or ')'. It returns nil without an error when the run is
// empty.
func (p *parser) parseAnd() (logbook.Expr, error) {
	var terms []logbook.Expr
	for {
		switch p.peek() {
		case eof, '|', ')':
			switch len(terms) {
			case 0:
				return nil, nil
			case 1:
				return terms[0], nil
			default:
				return logbook.And(terms), nil
			}
		}
		u, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		terms = append(terms, u)
	}
}

// parseUnary parses '!' unary, '(' expr ')' or a term. The caller has
// checked that the next rune can start one of them or is a bare word.
func (p *parser) parseUnary() (logbook.Expr, error) {
	start := p.pos
	switch p.peek() {
	case '!':
		p.pos++
		switch p.peek() {
		case eof, '|', ')':
			return nil, p.errorf(start, "dangling !")
		}
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return logbook.Not{X: x}, nil
	case '(':
		p.pos++
		switch p.peek() {
		case ')':
			return nil, p.errorf(start, "empty ()")
		case eof:
			return nil, p.errorf(start, "unbalanced (")
		}
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if e == nil {
			// Only '|' can stop an operand here; ')' and eof were checked.
			return nil, p.emptyOperand()
		}
		if p.peek() != ')' {
			return nil, p.errorf(start, "unbalanced (")
		}
		p.pos++
		return e, nil
	case '~':
		return p.parseTerm()
	default:
		return nil, p.errorf(start, "expected ~ term, found %q", p.src[start])
	}
}

// parseTerm parses '~' LETTER [value]; the next rune is '~'.
func (p *parser) parseTerm() (logbook.Expr, error) {
	start := p.pos
	p.pos++
	if p.pos >= len(p.src) || unicode.IsSpace(p.src[p.pos]) {
		return nil, p.errorf(start, "~ needs a term letter")
	}
	letter := p.src[p.pos]
	p.pos++
	name := "~" + string(letter)

	switch letter {
	case 'A':
		return logbook.All{}, nil
	case 'c', 'b', 'm', 'y', 'd':
	default:
		return nil, p.errorf(start, "unknown term %s", name)
	}

	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, p.errorf(start, "%s needs a value", name)
	}

	switch letter {
	case 'c':
		return logbook.CallContains{Call: value}, nil
	case 'b':
		return logbook.Filter{Bands: []string{value}}, nil
	case 'm':
		return logbook.ModeIs{Mode: value}, nil
	case 'y':
		return logbook.Filter{Tags: []string{value}}, nil
	default: // 'd'
		f, err := parseDateRange(value)
		if err != nil {
			return nil, fmt.Errorf("pattern: %s at column %d: %w", name, start+1, err)
		}
		return f, nil
	}
}

// parseValue parses an optional term value after optional whitespace: a
// quoted string or a bare word. It returns "" when there is none.
func (p *parser) parseValue() (string, error) {
	switch q := p.peek(); q {
	case eof:
		return "", nil
	case '"', '\'':
		return p.parseQuoted(q)
	}
	start := p.pos
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '|' {
			break
		}
		p.pos++
	}
	return string(p.src[start:p.pos]), nil
}

// parseQuoted parses a string quoted with q, which is the next rune. A
// backslash escapes the rune after it.
func (p *parser) parseQuoted(q rune) (string, error) {
	start := p.pos
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		p.pos++
		switch {
		case r == q:
			return b.String(), nil
		case r == '\\' && p.pos < len(p.src):
			b.WriteRune(p.src[p.pos])
			p.pos++
		default:
			b.WriteRune(r)
		}
	}
	return "", p.errorf(start, "unterminated %c quote", q)
}
