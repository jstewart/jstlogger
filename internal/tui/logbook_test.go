package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"jstlogger/internal/logbook"
)

// fakeLogbook is a logbook.Store whose methods are never called.
type fakeLogbook struct {
	logbook.Store
}

func TestWithLogbook(t *testing.T) {
	s := &fakeLogbook{}

	m := New(WithLogbook(s))

	assert.Same(t, s, m.logbook)
}

func TestNewWithoutLogbook(t *testing.T) {
	assert.Nil(t, New().logbook)
}
