package callbook_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	"jstlogger/internal/callbook/mock"
)

func TestCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	next := mock.NewMockLooker(ctrl)
	want := callbook.Result{Call: "W1AW", Name: "Hiram", Service: callbook.ServiceQRZ}
	next.EXPECT().Lookup(gomock.Any(), "w1aw").Return(want, nil).Times(1)

	c := &callbook.Cache{Next: next}
	got, err := c.Lookup(t.Context(), "w1aw")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Same call, different case and spacing: served from the cache.
	got, err = c.Lookup(t.Context(), " W1AW ")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCacheDoesNotCacheErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"not found", callbook.ErrNotFound},
		{"network", &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("down")}},
		{"rate limit", callbook.ErrRateLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			next := mock.NewMockLooker(ctrl)
			next.EXPECT().Lookup(gomock.Any(), "N0CALL").Return(callbook.Result{}, tt.err).Times(2)

			c := &callbook.Cache{Next: next}
			for range 2 {
				_, err := c.Lookup(t.Context(), "N0CALL")
				assert.ErrorIs(t, err, tt.err)
			}
		})
	}
}

func TestCacheTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	next := mock.NewMockLooker(ctrl)
	first := callbook.Result{Call: "W1AW", Name: "Old"}
	second := callbook.Result{Call: "W1AW", Name: "New"}
	gomock.InOrder(
		next.EXPECT().Lookup(gomock.Any(), "W1AW").Return(first, nil),
		next.EXPECT().Lookup(gomock.Any(), "W1AW").Return(second, nil),
	)

	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	c := &callbook.Cache{Next: next, TTL: time.Hour, Now: func() time.Time { return now }}

	got, err := c.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, first, got)

	now = now.Add(59 * time.Minute)
	got, err = c.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, first, got, "still fresh")

	now = now.Add(2 * time.Minute)
	got, err = c.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, second, got, "expired")
}

func TestCacheZeroTTLNeverExpires(t *testing.T) {
	ctrl := gomock.NewController(t)
	next := mock.NewMockLooker(ctrl)
	want := callbook.Result{Call: "W1AW"}
	next.EXPECT().Lookup(gomock.Any(), "W1AW").Return(want, nil).Times(1)

	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	c := &callbook.Cache{Next: next, Now: func() time.Time { return now }}
	_, err := c.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)

	now = now.Add(24 * 365 * time.Hour)
	got, err := c.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCacheConcurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	next := mock.NewMockLooker(ctrl)
	next.EXPECT().Lookup(gomock.Any(), gomock.Any()).Return(callbook.Result{Call: "W1AW"}, nil).AnyTimes()

	c := &callbook.Cache{Next: next, TTL: time.Minute}
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			got, err := c.Lookup(t.Context(), "W1AW")
			assert.NoError(t, err)
			assert.Equal(t, "W1AW", got.Call)
		})
	}
	wg.Wait()
}
