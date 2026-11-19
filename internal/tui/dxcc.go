package tui

import (
	"fmt"
	"strconv"

	"jstlogger/internal/dxcc"
	"jstlogger/internal/qsoentry"
)

// DXCCLookup resolves callsigns to DXCC entities. *dxcc.Table satisfies
// it.
type DXCCLookup interface {
	// Lookup returns the DXCC entity for call, and whether call is in one.
	Lookup(call string) (dxcc.Entity, bool)
}

// WithDXCC fills in, on the New QSO screen, the DXCC entity, continent and
// zones of the call typed, as looked up in l, and shows the entity next to
// the call.
func WithDXCC(l DXCCLookup) Option {
	return func(m *Model) {
		m.dxcc = l
	}
}

// dxccFields are the fields filled in from the call's DXCC entity, in the
// order entityValues returns their values.
var dxccFields = [4]qsoentry.Field{qsoentry.DXCC, qsoentry.Cont, qsoentry.CQZ, qsoentry.ITUZ}

// entityValues returns the values of dxccFields for call, as looked up in l,
// or empty values when l is nil or call is in no entity.
func entityValues(l DXCCLookup, call string) [4]string {
	if l == nil {
		return [4]string{}
	}
	ent, ok := l.Lookup(call)
	if !ok {
		return [4]string{}
	}
	return [4]string{
		strconv.Itoa(ent.DXCC), ent.Continent,
		strconv.Itoa(ent.CQZone), strconv.Itoa(ent.ITUZone),
	}
}

// fillDXCC fills in the DXCC entity, continent and zones of the call, each
// in its field if it is empty or holds the value last filled in. A call in
// no entity clears the values filled in. It does nothing without a DXCC
// lookup.
func (e *entry) fillDXCC() {
	if e.dxccLookup == nil {
		return
	}
	derived := entityValues(e.dxccLookup, e.value(qsoentry.Call))
	for i, f := range dxccFields {
		if v := e.input(f).Value(); v == "" || v == e.dxcc[i] {
			e.input(f).SetValue(derived[i])
		}
	}
	e.dxcc = derived
}

// entryDXCC returns the DXCC entity of the call typed into the New QSO
// screen, styled: its name, continent and zones. It is "" without a DXCC
// lookup or while the call is in no entity.
func (m Model) entryDXCC() string {
	if m.dxcc == nil {
		return ""
	}
	ent, ok := m.dxcc.Lookup(m.entry.value(qsoentry.Call))
	if !ok {
		return ""
	}
	return m.styles.muted.Render(fmt.Sprintf("%s · %s · CQ %d · ITU %d", ent.Name, ent.Continent, ent.CQZone, ent.ITUZone))
}
