package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cbmock "jstlogger/internal/callbook/mock"
)

func TestBulkSyncKeyDefaults(t *testing.T) {
	k := defaultIndexKeys(true)

	assert.Equal(t, []string{"c"}, k.BulkSync.Keys())
	assert.Equal(t, ";c", k.BulkSync.Help().Key)
	assert.Equal(t, "sync marked with callbook", k.BulkSync.Help().Desc)
	assert.True(t, k.BulkSync.Enabled())
}

func TestBulkSyncKeyDisabled(t *testing.T) {
	assert.False(t, defaultIndexKeys(false).BulkSync.Enabled())
}

func TestBulkSyncKeyPagerCopiesIndex(t *testing.T) {
	ik := defaultIndexKeys(true)
	pk := defaultPagerKeys(false, ik)

	assert.Equal(t, ik.BulkSync.Keys(), pk.BulkSync.Keys())
	assert.Equal(t, ik.BulkSync.Help(), pk.BulkSync.Help())
	assert.True(t, pk.BulkSync.Enabled())

	assert.False(t, defaultPagerKeys(false, defaultIndexKeys(false)).BulkSync.Enabled())
}

func TestBulkSyncKeyInPagerHelp(t *testing.T) {
	pk := defaultPagerKeys(false, defaultIndexKeys(true))

	var found bool
	for _, s := range pk.sections() {
		if s.title != "marks" {
			continue
		}
		for i, b := range s.keys {
			if b.Help().Desc == "sync marked with callbook" {
				found = true
				assert.Equal(t, "untag marked", s.keys[i-1].Help().Desc)
			}
		}
	}
	assert.True(t, found, "pager marks help lists BulkSync")
}

func TestBulkSyncKeyInIndexHelp(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	m, _ := newIndexModel(t, 3, 200, 30, WithCallbook(l))

	assert.Regexp(t, `;c +sync marked with callbook`, helpText(m))
}
