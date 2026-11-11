package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/callsign"
)

// EQSLLookup reports whether a callsign is an eQSL Authenticity Guaranteed
// member.
type EQSLLookup interface {
	// IsAG reports whether call is an AG member.
	IsAG(ctx context.Context, call string) (bool, error)
}

// WithEQSL shows, on the pager's QSO tab, whether the QSO's callsign is an
// eQSL Authenticity Guaranteed member, as looked up in l.
func WithEQSL(l EQSLLookup) Option {
	return func(m *Model) {
		m.eqsl = l
	}
}

// EQSLSynced reports that a background sync of the eQSL AG member list
// finished. Err is nil when it succeeded.
type EQSLSynced struct {
	Err error
}

// eqslCache is the pager's eQSL lookup for one base callsign. call is the
// base callsign that ag and err are for; pending is the base callsign being
// looked up.
type eqslCache struct {
	call    string
	pending string
	ag      bool
	err     error
}

// eqslLookupMsg carries the result of an eQSL lookup for the pager.
type eqslLookupMsg struct {
	call string
	ag   bool
	err  error
}

// eqslLookupCmd returns a command that looks up the base callsign call in
// l.
func eqslLookupCmd(l EQSLLookup, call string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		ag, err := l.IsAG(ctx, call)
		if err != nil {
			err = fmt.Errorf("look up %s on eQSL: %w", call, err)
		}
		return eqslLookupMsg{call: call, ag: ag, err: err}
	}
}

// fetchEQSL returns a command looking up the base callsign of the QSO under
// the index cursor, unless it has been looked up or is being looked up.
func (m *Model) fetchEQSL() tea.Cmd {
	q, ok := m.idx.current()
	if !m.pager.open || m.eqsl == nil || !ok {
		return nil
	}
	base, c := callsign.Base(q.Call), &m.pager.eqsl
	if base == "" || base == c.call || base == c.pending {
		return nil
	}
	c.pending = base
	return eqslLookupCmd(m.eqsl, base)
}

// applyEQSL keeps an eQSL lookup for the pager. A lookup for a callsign the
// cursor has moved off, or one landing after the pager closed, is dropped.
// It reports whether the lookup was kept.
func (p *pager) applyEQSL(msg eqslLookupMsg, current string) bool {
	if msg.call == p.eqsl.pending {
		p.eqsl.pending = ""
	}
	if !p.open || msg.call != current {
		return false
	}
	p.eqsl.call, p.eqsl.ag, p.eqsl.err = msg.call, msg.ag, msg.err
	return true
}

// applyEQSLSynced records the outcome of an eQSL sync. A successful sync
// forgets the pager's lookup, so the QSO shown is looked up again.
func (m *Model) applyEQSLSynced(msg EQSLSynced) {
	if msg.Err != nil {
		m.eqslErr = fmt.Errorf("eQSL sync failed: %w", msg.Err)
		return
	}
	m.eqslErr = nil
	m.pager.eqsl = eqslCache{}
}

// eqslStatus is what the QSO tab shows of a callsign's eQSL AG membership.
type eqslStatus struct {
	// enabled reports whether an eQSL lookup is configured; without one
	// the QSO tab has no eQSL AG row.
	enabled bool
	// loaded reports whether the lookup succeeded; ag is its result.
	loaded bool
	ag     bool
	// failed reports whether the lookup failed.
	failed bool
}

// eqslStatus returns the eQSL AG status of the QSO with call, from the
// pager's lookup. It does no I/O.
func (m Model) eqslStatus(call string) eqslStatus {
	if m.eqsl == nil {
		return eqslStatus{}
	}
	c := m.pager.eqsl
	s := eqslStatus{enabled: true}
	if base := callsign.Base(call); base == "" || c.call != base {
		return s
	}
	if c.err != nil {
		s.failed = true
		return s
	}
	s.loaded, s.ag = true, c.ag
	return s
}

// badgeStyle returns the style of the [E] badge: bright for an AG member,
// dimmed for a callsign known not to be one, and as without a lookup while
// membership is not known.
func (s eqslStatus) badgeStyle(st styles) lipgloss.Style {
	switch {
	case !s.loaded:
		return st.eQSL
	case s.ag:
		return st.eQSLAG
	default:
		return st.eQSLNotAG
	}
}

// text describes the callsign's AG membership, or "" while it is not known
// or the callsign is not a member.
func (s eqslStatus) text() string {
	switch {
	case s.failed:
		return "lookup failed"
	case s.loaded && s.ag:
		return "yes"
	default:
		return ""
	}
}
