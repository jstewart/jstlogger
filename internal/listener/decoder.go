package listener

import (
	"errors"
	"fmt"
)

// Decoder turns one received packet into events.
type Decoder interface {
	// Decode returns the events in one packet; packets it has no use for
	// yield no events and a nil error.
	Decode(packet []byte) ([]any, error)
}

// ProtocolWSJTX names the WSJT-X UDP protocol, also spoken by JTDX.
const ProtocolWSJTX = "wsjtx"

// Errors reported by NewDecoder, decoders and the Listener.
var (
	// ErrUnknownProtocol means NewDecoder has no decoder for the protocol.
	ErrUnknownProtocol = errors.New("listener: unknown protocol")
	// ErrBadMagic means a packet does not start with the protocol's magic number.
	ErrBadMagic = errors.New("listener: bad magic number")
	// ErrTruncated means a packet ended before a field it declares.
	ErrTruncated = errors.New("listener: truncated packet")
	// ErrForeignSender means a packet came from a host other than this one.
	ErrForeignSender = errors.New("listener: sender is not local")
	// ErrNotRunning means Send was called while the Listener was not
	// running.
	ErrNotRunning = errors.New("listener: not running")
)

// NewDecoder returns the decoder for protocol. Unknown protocols return an
// error wrapping ErrUnknownProtocol.
func NewDecoder(protocol string) (Decoder, error) {
	switch protocol {
	case ProtocolWSJTX:
		return WSJTX{}, nil
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownProtocol, protocol)
}
