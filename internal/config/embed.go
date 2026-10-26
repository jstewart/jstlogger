package config

import (
	_ "embed"
	"fmt"
	"io"
)

// DefaultTemplate is the minimal config file written on first run. It holds
// only the required options; every other default lives in code.
//
//go:embed default.toml
var DefaultTemplate []byte

// FullExample is a fully commented config containing every option, as printed
// by --print-default-config.
//
//go:embed full.toml
var FullExample []byte

// WriteFullExample writes FullExample to w.
func WriteFullExample(w io.Writer) error {
	if _, err := w.Write(FullExample); err != nil {
		return fmt.Errorf("writing full example config: %w", err)
	}
	return nil
}
