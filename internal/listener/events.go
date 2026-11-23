package listener

import "jstlogger/internal/logbook"

// QSOReceived reports a QSO another program logged.
type QSOReceived struct {
	// Source names the program that sent the QSO, such as the WSJT-X client
	// ID ("WSJT-X", "JTDX").
	Source string
	// QSO is the logged contact. It is not normalized.
	QSO logbook.QSO
	// Dropped holds a *logbook.FieldError for each optional field whose
	// value could not be parsed and was left out of QSO, so a typo in a
	// free-text field such as TX_PWR does not lose the QSO.
	Dropped []error
}

// PacketDropped reports a packet the listener could not use: one that failed
// to decode, or that came from a sender that isn't local.
type PacketDropped struct {
	// From is the sender's address.
	From string
	// Err is why the packet was dropped.
	Err error
}
