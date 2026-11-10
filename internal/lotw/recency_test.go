package lotw

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecencyOf(t *testing.T) {
	now := utc(2026, 11, 8, 12, 0, 0)
	tests := []struct {
		name string
		last time.Time
		want Recency
	}{
		{"zero is unknown", time.Time{}, Unknown},
		{"just now", now, Recent},
		{"in the future", now.Add(time.Hour), Recent},
		{"exactly 30 days", now.Add(-RecentWithin), Recent},
		{"just over 30 days", now.Add(-RecentWithin - time.Second), Active},
		{"exactly a year", now.Add(-ActiveWithin), Active},
		{"just over a year", now.Add(-ActiveWithin - time.Second), Inactive},
		{"long ago", utc(2001, 1, 1, 0, 0, 0), Inactive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RecencyOf(tt.last, now))
		})
	}
}

func TestRecencyZeroValueIsUnknown(t *testing.T) {
	var r Recency
	assert.Equal(t, Unknown, r)
}

func TestDaysAgo(t *testing.T) {
	now := utc(2026, 11, 8, 12, 0, 0)
	tests := []struct {
		name string
		last time.Time
		want int
	}{
		{"now", now, 0},
		{"future is never negative", now.Add(48 * time.Hour), 0},
		{"just under a day", now.Add(-24*time.Hour + time.Second), 0},
		{"exactly a day", now.Add(-24 * time.Hour), 1},
		{"a day and a half floors", now.Add(-36 * time.Hour), 1},
		{"thirty days", now.Add(-RecentWithin), 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DaysAgo(tt.last, now))
		})
	}
}
