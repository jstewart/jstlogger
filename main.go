package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "jstlogger: %v\n", err)
		os.Exit(1)
	}
}
