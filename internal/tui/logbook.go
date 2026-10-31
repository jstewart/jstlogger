package tui

import "jstlogger/internal/logbook"

// WithLogbook gives the UI the logbook that stores QSOs.
func WithLogbook(s logbook.Store) Option {
	return func(m *Model) {
		m.logbook = s
	}
}
