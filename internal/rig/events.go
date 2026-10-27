// Package rig controls a radio through Hamlib's rigctld. It polls the rig,
// reports changes as events and accepts commands such as setting the
// frequency or mode. It knows nothing about the terminal UI.
package rig

import "time"

// State is the rig state that jstlogger tracks.
type State struct {
	// Freq is the dial frequency in hertz.
	Freq int64
	// Mode is the Hamlib mode name, such as "USB" or "PKTUSB".
	Mode string
	// Passband is the passband width in hertz; 0 means the rig's default.
	Passband int
}

// StateChanged is emitted when a poll finds the rig state differs from the
// last state emitted.
type StateChanged struct {
	State State
}

// ConnStatus is the state of the connection to rigctld.
type ConnStatus int

// Connection states reported in ConnectionChanged.
const (
	// Connecting means a connection attempt is in progress.
	Connecting ConnStatus = iota
	// Connected means rigctld is reachable and the rig state is current.
	Connected
	// Disconnected means the connection was lost or could not be made; the
	// last reported State is stale until the next Connected.
	Disconnected
)

// String returns the lowercase name of s.
func (s ConnStatus) String() string {
	switch s {
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Disconnected:
		return "disconnected"
	}
	return "unknown"
}

// ConnectionChanged is emitted when the connection status changes.
type ConnectionChanged struct {
	Status ConnStatus
	// Err is the reason for a Disconnected status, if any.
	Err error
	// RetryIn is the delay before the next connection attempt when Status is
	// Disconnected.
	RetryIn time.Duration
}

// CommandResult is emitted after a command sent to the rig completes.
type CommandResult struct {
	// Command describes the command, such as "set_freq 14074000".
	Command string
	// Err is non-nil if the command failed.
	Err error
}
