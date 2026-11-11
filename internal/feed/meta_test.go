package feed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetaStale(t *testing.T) {
	syncedAt := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	maxAge := 7 * 24 * time.Hour
	tests := []struct {
		name string
		meta Meta
		now  time.Time
		want bool
	}{
		{"never synced", Meta{}, syncedAt, true},
		{"just synced", Meta{SyncedAt: syncedAt}, syncedAt, false},
		{"exactly maxAge old", Meta{SyncedAt: syncedAt}, syncedAt.Add(maxAge), false},
		{"older than maxAge", Meta{SyncedAt: syncedAt}, syncedAt.Add(maxAge + time.Second), true},
		{"synced in the future", Meta{SyncedAt: syncedAt}, syncedAt.Add(-time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.meta.Stale(tt.now, maxAge))
		})
	}
}

func TestTimeRoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 8, 10, 5, 7, 999, time.FixedZone("EST", -5*3600))

	text := FormatTime(at)

	assert.Equal(t, "2026-11-08T15:05:07Z", text, "UTC, second precision")
	got, err := ParseTime(text)
	require.NoError(t, err)
	assert.True(t, at.Truncate(time.Second).Equal(got))
	assert.Equal(t, time.UTC, got.Location())
}

func TestParseTimeRejectsGarbage(t *testing.T) {
	_, err := ParseTime("yesterday")

	require.Error(t, err)
}
