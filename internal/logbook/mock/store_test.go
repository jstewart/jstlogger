package mock

import "jstlogger/internal/logbook"

// MockStore must keep up with logbook.Store; regenerate with `just generate`.
var _ logbook.Store = (*MockStore)(nil)
