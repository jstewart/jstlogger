package listener

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleADIF is LoggedADIF text in the form WSJT-X sends.
const sampleADIF = "\n<adif_ver:5>3.1.0\n<programid:6>WSJT-X\n<EOH>\n" +
	"<call:4>K1AB <gridsquare:4>FN42 <mode:3>FT8 <rst_sent:3>-10 <rst_rcvd:3>-12 " +
	"<qso_date:8>20261123 <time_on:6>130102 <qso_date_off:8>20261123 <time_off:6>130215 " +
	"<band:3>20m <freq:9>14.075123 <station_callsign:5>W1XYZ <my_gridsquare:6>FN42ab <EOR>"

// packet builds a QDataStream byte sequence.
type packet []byte

// u32 appends a big-endian quint32.
func (p packet) u32(v uint32) packet { return binary.BigEndian.AppendUint32(p, v) }

// str appends a QByteArray holding s.
func (p packet) str(s string) packet { return append(p.u32(uint32(len(s))), s...) }

// null appends a null QByteArray.
func (p packet) null() packet { return p.u32(0xFFFFFFFF) }

// header starts a packet with the magic, schema, type and client Id.
func header(msgType uint32, id string) packet {
	return packet{}.u32(wsjtxMagic).u32(2).u32(msgType).str(id)
}

func heartbeat() packet {
	return header(0, "WSJT-X").u32(3).str("2.7.0").str("abc123")
}

func loggedADIF(id, text string) packet { return header(12, id).str(text) }

func TestWSJTXDecode(t *testing.T) {
	logged := loggedADIF("WSJT-X", sampleADIF)
	idEnd := 12 + 4 + len("WSJT-X")

	tests := []struct {
		name    string
		packet  []byte
		wantErr error
		wantAny bool // expect an error, of no particular sentinel
		want    int  // number of QSOReceived events
		source  string
	}{
		{name: "heartbeat", packet: heartbeat()},
		{name: "logged ADIF", packet: logged, want: 1, source: "WSJT-X"},
		{name: "null id", packet: packet{}.u32(wsjtxMagic).u32(2).u32(12).null().str(sampleADIF), want: 1, source: ""},
		{name: "QSO logged ignored", packet: header(5, "WSJT-X").u32(0xDEAD)},
		{name: "unknown type ignored", packet: header(99, "JTDX")},
		{name: "empty ADIF", packet: loggedADIF("WSJT-X", "")},
		{name: "null ADIF", packet: header(12, "WSJT-X").null()},
		{name: "header only ADIF", packet: loggedADIF("WSJT-X", "\n<adif_ver:5>3.1.0\n<EOH>\n")},
		{name: "bad magic", packet: packet{}.u32(0x12345678).u32(2).u32(12).str("X"), wantErr: ErrBadMagic},
		{name: "empty packet", packet: nil, wantErr: ErrTruncated},
		{name: "cut in header", packet: logged[:6], wantErr: ErrTruncated},
		{name: "cut before id length", packet: logged[:12], wantErr: ErrTruncated},
		{name: "cut in id", packet: logged[:idEnd-2], wantErr: ErrTruncated},
		{name: "cut in ADIF length", packet: logged[:idEnd+2], wantErr: ErrTruncated},
		{name: "cut in ADIF bytes", packet: logged[:len(logged)-5], wantErr: ErrTruncated},
		{name: "huge length", packet: header(12, "WSJT-X").u32(0xFFFFFFF0), wantErr: ErrTruncated},
		{name: "missing call", packet: loggedADIF("WSJT-X", "<band:3>20m <mode:3>FT8 <qso_date:8>20261123 <time_on:6>130102 <EOR>"), wantAny: true},
		{name: "malformed ADIF", packet: loggedADIF("WSJT-X", "<call:99>K1AB <EOR>"), wantAny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs, err := WSJTX{}.Decode(tt.packet)
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, evs)
				return
			case tt.wantAny:
				require.Error(t, err)
				assert.Empty(t, evs)
				return
			}
			require.NoError(t, err)
			require.Len(t, evs, tt.want)
			for _, ev := range evs {
				got, ok := ev.(QSOReceived)
				require.True(t, ok, "event %T", ev)
				assert.Equal(t, tt.source, got.Source)
				assert.Equal(t, "K1AB", got.QSO.Call)
				assert.Equal(t, "20m", got.QSO.Band)
				assert.Equal(t, "FT8", got.QSO.Mode)
				assert.Equal(t, time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), got.QSO.StartUTC)
			}
		})
	}
}

func TestWSJTXDecodeMultipleRecords(t *testing.T) {
	text := "<call:4>K1AB <band:3>20m <mode:3>FT8 <qso_date:8>20261123 <time_on:6>130102 <EOR>" +
		"<call:4>K2CD <band:3>40m <mode:3>FT4 <qso_date:8>20261123 <time_on:6>131000 <EOR>"
	evs, err := WSJTX{}.Decode(loggedADIF("JTDX", text))
	require.NoError(t, err)
	require.Len(t, evs, 2)
	assert.Equal(t, "K1AB", evs[0].(QSOReceived).QSO.Call)
	assert.Equal(t, "K2CD", evs[1].(QSOReceived).QSO.Call)
	assert.Equal(t, "JTDX", evs[1].(QSOReceived).Source)
}

func TestNewDecoder(t *testing.T) {
	d, err := NewDecoder(ProtocolWSJTX)
	require.NoError(t, err)
	assert.IsType(t, WSJTX{}, d)

	_, err = NewDecoder("n1mm")
	require.ErrorIs(t, err, ErrUnknownProtocol)
	assert.False(t, errors.Is(err, ErrBadMagic))
}
