package listener

import (
	"net/netip"

	"jstlogger/internal/logbook"
)

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

// StatusReceived reports the state of the program that sent it, such as a
// WSJT-X Status message, sent when its dial frequency or mode changes.
type StatusReceived struct {
	// Source names the program, as QSOReceived's Source does.
	Source string
	// From is the sender's address, where replies to it are sent. The
	// Listener fills it in.
	From netip.AddrPort
	// DialHz is the rig's dial frequency in Hz.
	DialHz uint64
	// Mode is the mode the program is decoding, such as "FT8".
	Mode string
}

// withFrom implements addressed.
func (s StatusReceived) withFrom(from netip.AddrPort) any {
	s.From = from
	return s
}

// DecodeReceived reports a message the program that sent it decoded, such
// as a WSJT-X Decode message.
type DecodeReceived struct {
	// Source names the program, as QSOReceived's Source does.
	Source string
	// From is the sender's address, where replies to it are sent. The
	// Listener fills it in.
	From netip.AddrPort
	// Message is the decoded text, such as "CQ K1ABC FN42".
	Message string
}

// withFrom implements addressed.
func (d DecodeReceived) withFrom(from netip.AddrPort) any {
	d.From = from
	return d
}

// addressed is an event that carries its sender's address, which the
// Listener fills in, as decoders see only the packet.
type addressed interface {
	// withFrom returns the event with its sender's address set to from.
	withFrom(from netip.AddrPort) any
}
