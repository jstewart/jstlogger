package listener

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"unicode/utf8"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// wsjtxMagic starts every WSJT-X packet.
const wsjtxMagic uint32 = 0xADBCCBDA

// WSJT-X message types.
const (
	// wsjtxStatus carries the sender's dial frequency, mode and more.
	wsjtxStatus uint32 = 1
	// wsjtxDecode carries one decoded message.
	wsjtxDecode uint32 = 2
	// wsjtxLoggedADIF carries a logged QSO as ADIF.
	wsjtxLoggedADIF uint32 = 12
	// wsjtxHighlightCallsign asks the receiver to highlight a callsign.
	wsjtxHighlightCallsign uint32 = 13
)

// wsjtxSchema is the schema number of the packets this package sends:
// QDataStream's Qt 5.0 format, which every WSJT-X 2.x and JTDX reads.
const wsjtxSchema uint32 = 2

// qNull is the QByteArray length that marks a null array.
const qNull uint32 = 0xFFFFFFFF

// WSJTX decodes the WSJT-X UDP protocol. It reports Status messages as
// StatusReceived, Decode messages as DecodeReceived and LoggedADIF
// messages as QSOReceived events, and ignores every other message type,
// including QSOLogged, which repeats the QSO sent in LoggedADIF.
type WSJTX struct{}

// Decode returns the events in one WSJT-X message: a StatusReceived for a
// Status message, a DecodeReceived for a Decode message, a QSOReceived for
// each record in a LoggedADIF message, and none for any other message.
func (WSJTX) Decode(packet []byte) ([]any, error) {
	r := qreader{buf: packet}
	magic, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read magic: %w", err)
	}
	if magic != wsjtxMagic {
		return nil, fmt.Errorf("listener: wsjtx: %w %#08x", ErrBadMagic, magic)
	}
	if _, err := r.u32(); err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read schema: %w", err)
	}
	msgType, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read message type: %w", err)
	}
	switch msgType {
	case wsjtxStatus, wsjtxDecode, wsjtxLoggedADIF:
	default:
		return nil, nil
	}
	id, err := r.utf8()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read id: %w", err)
	}
	switch msgType {
	case wsjtxStatus:
		return decodeStatus(&r, id)
	case wsjtxDecode:
		return decodeDecode(&r, id)
	}
	return decodeLoggedADIF(&r, id)
}

// decodeStatus reads the rest of a Status message from id: its dial
// frequency and mode. The fields after them are ignored.
func decodeStatus(r *qreader, id string) ([]any, error) {
	dial, err := r.u64()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: status from %q: read dial frequency: %w", id, err)
	}
	mode, err := r.utf8()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: status from %q: read mode: %w", id, err)
	}
	return []any{StatusReceived{Source: id, DialHz: dial, Mode: mode}}, nil
}

// decodeDecode reads the rest of a Decode message from id up to its
// message text. The fields after it are ignored.
func decodeDecode(r *qreader, id string) ([]any, error) {
	// New (bool), time (quint32), SNR (qint32), delta time (double) and
	// delta frequency (quint32) come before the mode and message.
	if err := r.skip(1 + 4 + 4 + 8 + 4); err != nil {
		return nil, fmt.Errorf("listener: wsjtx: decode from %q: %w", id, err)
	}
	if _, err := r.utf8(); err != nil {
		return nil, fmt.Errorf("listener: wsjtx: decode from %q: read mode: %w", id, err)
	}
	msg, err := r.utf8()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: decode from %q: read message: %w", id, err)
	}
	return []any{DecodeReceived{Source: id, Message: msg}}, nil
}

// decodeLoggedADIF reads the rest of a LoggedADIF message from id and
// returns a QSOReceived for each of its records.
func decodeLoggedADIF(r *qreader, id string) ([]any, error) {
	text, err := r.utf8()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read ADIF: %w", err)
	}
	if text == "" {
		return nil, nil
	}
	file, err := adif.Parse(strings.NewReader(text))
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: parse ADIF from %q: %w", id, err)
	}
	events := make([]any, 0, len(file.Records))
	for i, rec := range file.Records {
		for j, f := range rec {
			rec[j].Value = latin1ToUTF8(f.Value)
		}
		q, dropped, err := qsoFromRecord(rec)
		if err != nil {
			return nil, fmt.Errorf("listener: wsjtx: record %d from %q: %w", i+1, id, err)
		}
		events = append(events, QSOReceived{Source: id, QSO: q, Dropped: dropped})
	}
	return events, nil
}

// WSJTXHighlightCallsign returns a HighlightCallsign message for the
// program with the client ID id: highlight call in its Band Activity
// window with the background bg and the foreground fg. A nil color leaves
// that color as the program's default, so two nil colors clear the
// highlight. With last set, only the newest decode of call is
// highlighted; otherwise every one is, including later ones.
func WSJTXHighlightCallsign(id, call string, bg, fg color.Color, last bool) []byte {
	b := binary.BigEndian.AppendUint32(nil, wsjtxMagic)
	b = binary.BigEndian.AppendUint32(b, wsjtxSchema)
	b = binary.BigEndian.AppendUint32(b, wsjtxHighlightCallsign)
	b = appendUTF8(b, id)
	b = appendUTF8(b, call)
	b = appendQColor(b, bg)
	b = appendQColor(b, fg)
	if last {
		return append(b, 1)
	}
	return append(b, 0)
}

// appendUTF8 appends s as a QByteArray.
func appendUTF8(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s))) // a callsign or client ID is far shorter than 4 GiB
	return append(b, s...)
}

// QColor color specs.
const (
	qColorInvalid = 0
	qColorRGB     = 1
)

// appendQColor appends c as a QColor: its spec, then alpha, red, green and
// blue as 16-bit values and a padding word. A nil c is an invalid QColor.
func appendQColor(b []byte, c color.Color) []byte {
	if c == nil {
		// An invalid color is streamed with an opaque alpha and zero
		// components, as QColor() is.
		return append(b, qColorInvalid, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	r, g, bl, a := c.RGBA()
	b = append(b, qColorRGB)
	for _, v := range []uint32{a, r, g, bl, 0} {
		b = binary.BigEndian.AppendUint16(b, uint16(v)) // color.Color components are 16-bit
	}
	return b
}

// requiredFields are the ADIF fields a QSO cannot do without, so a bad value
// in one of them drops the whole record rather than just the field.
var requiredFields = []string{"CALL", "QSO_DATE", "TIME_ON"}

// qsoFromRecord converts rec to a QSO. A field other than requiredFields
// whose value cannot be parsed is left out, and its *logbook.FieldError is
// returned in dropped.
func qsoFromRecord(rec adif.Record) (q logbook.QSO, dropped []error, err error) {
	for {
		q, err = logbook.QSOFromRecord(rec)
		var fe *logbook.FieldError
		if err == nil || !errors.As(err, &fe) || slices.Contains(requiredFields, fe.Field) {
			return q, dropped, err
		}
		kept := slices.DeleteFunc(slices.Clone(rec), func(f adif.Field) bool {
			return strings.EqualFold(strings.TrimSpace(f.Name), fe.Field)
		})
		if len(kept) == len(rec) {
			return q, dropped, err // not found; can't happen, but never loop
		}
		rec = kept
		dropped = append(dropped, fe)
	}
}

// latin1ToUTF8 converts s from Latin-1 to UTF-8 when it is not already valid
// UTF-8. WSJT-X sends its ADIF as Latin-1 bytes, though its protocol
// documents UTF-8; JTDX and MSHV may send UTF-8, which is kept as is.
func latin1ToUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	b := make([]byte, 0, 2*len(s))
	for i := range len(s) {
		b = utf8.AppendRune(b, rune(s[i]))
	}
	return string(b)
}

// qreader reads big-endian Qt QDataStream values from a byte slice.
type qreader struct {
	buf []byte
}

// u32 reads a quint32.
func (r *qreader) u32() (uint32, error) {
	if len(r.buf) < 4 {
		return 0, ErrTruncated
	}
	v := binary.BigEndian.Uint32(r.buf)
	r.buf = r.buf[4:]
	return v, nil
}

// u64 reads a quint64.
func (r *qreader) u64() (uint64, error) {
	if len(r.buf) < 8 {
		return 0, ErrTruncated
	}
	v := binary.BigEndian.Uint64(r.buf)
	r.buf = r.buf[8:]
	return v, nil
}

// skip skips n bytes.
func (r *qreader) skip(n int) error {
	if len(r.buf) < n {
		return ErrTruncated
	}
	r.buf = r.buf[n:]
	return nil
}

// utf8 reads a QByteArray holding UTF-8 text. A null array reads as "".
func (r *qreader) utf8() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	if n == qNull {
		return "", nil
	}
	if uint64(n) > uint64(len(r.buf)) {
		return "", ErrTruncated
	}
	s := string(r.buf[:n])
	r.buf = r.buf[n:]
	return s, nil
}
