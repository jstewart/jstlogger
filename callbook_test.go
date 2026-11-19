package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/callbook"
	"jstlogger/internal/config"
)

func TestNewCallbookWithoutServices(t *testing.T) {
	assert.Nil(t, newCallbook(config.Callbook{CacheTTL: time.Hour}))
}

func TestNewCallbook(t *testing.T) {
	cfg := config.Callbook{
		Services: []string{config.CallbookHamQTH, config.CallbookQRZ},
		CacheTTL: 2 * time.Hour,
		QRZ:      config.QRZAccount{Username: "w1aw", Password: "qrz-pw", FullName: true},
		HamQTH:   config.CallbookAccount{Username: "ve3abc", Password: "hamqth-pw"},
	}

	l := newCallbook(cfg)

	cache, ok := l.(*callbook.Cache)
	require.True(t, ok, "lookup is %T, want *callbook.Cache", l)
	assert.Equal(t, 2*time.Hour, cache.TTL)
	chain, ok := cache.Next.(callbook.Chain)
	require.True(t, ok, "cache wraps %T, want callbook.Chain", cache.Next)
	require.Len(t, chain, 2)

	hamqth, ok := chain[0].(*callbook.HamQTH)
	require.True(t, ok, "first service is %T, want *callbook.HamQTH", chain[0])
	assert.Equal(t, "ve3abc", hamqth.Username)
	assert.Equal(t, "hamqth-pw", hamqth.Password)
	require.NotNil(t, hamqth.Client)
	assert.Equal(t, callbookTimeout, hamqth.Client.Timeout)

	qrz, ok := chain[1].(*callbook.QRZ)
	require.True(t, ok, "second service is %T, want *callbook.QRZ", chain[1])
	assert.Equal(t, "w1aw", qrz.Username)
	assert.Equal(t, "qrz-pw", qrz.Password)
	assert.True(t, qrz.FullName)
	require.NotNil(t, qrz.Client)
	assert.Equal(t, callbookTimeout, qrz.Client.Timeout)
}
