package listener

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// wsjtxMagic starts every WSJT-X packet.
const wsjtxMagic uint32 = 0xADBCCBDA

// wsjtxLoggedADIF is the message type that carries a logged QSO as ADIF.
const wsjtxLoggedADIF uint32 = 12

// qNull is the QByteArray length that marks a null array.
const qNull uint32 = 0xFFFFFFFF

// WSJTX decodes the WSJT-X UDP protocol. It reports LoggedADIF messages as
// QSOReceived events and ignores every other message type, including
// QSOLogged, which repeats the QSO sent in LoggedADIF.
type WSJTX struct{}

// Decode returns a QSOReceived for each record in a LoggedADIF message and
// no events for any other message.
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
	if msgType != wsjtxLoggedADIF {
		return nil, nil
	}
	id, err := r.utf8()
	if err != nil {
		return nil, fmt.Errorf("listener: wsjtx: read id: %w", err)
	}
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
		q, err := logbook.QSOFromRecord(rec)
		if err != nil {
			return nil, fmt.Errorf("listener: wsjtx: record %d from %q: %w", i+1, id, err)
		}
		events = append(events, QSOReceived{Source: id, QSO: q})
	}
	return events, nil
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
