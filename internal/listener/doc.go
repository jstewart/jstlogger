// Package listener receives UDP packets from other amateur radio programs,
// such as WSJT-X, and turns them into events such as a logged QSO. Each
// packet is handed to a per-protocol Decoder. The package knows nothing about
// the terminal UI, configuration or storage.
package listener
