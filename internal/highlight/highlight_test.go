package highlight

import (
	"errors"
	"image/color"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/logbook"
	"jstlogger/internal/logbook/mock"
)

func TestCaller(t *testing.T) {
	tests := []struct{ msg, want string }{
		{"CQ K1ABC FN42", "K1ABC"},
		{"CQ DX K1ABC FN42", "K1ABC"},
		{"CQ POTA K1ABC FN42", "K1ABC"},
		{"CQ 290 K1ABC FN42", "K1ABC"},
		{"CQ K1ABC", "K1ABC"},
		{"cq k1abc fn42", "K1ABC"},
		{"QRZ K1ABC", "K1ABC"},
		{"W9XYZ K1ABC -10", "K1ABC"},
		{"W9XYZ K1ABC R-10", "K1ABC"},
		{"W9XYZ K1ABC RR73", "K1ABC"},
		{"W9XYZ VE3/K1ABC 73", "VE3/K1ABC"},
		{"W9XYZ K1ABC/P FN42", "K1ABC/P"},
		{"<W9XYZ> K1ABC FN42", "K1ABC"},
		{"W9XYZ <K1ABC> -10", "K1ABC"},
		{"W9XYZ <...> -10", ""},
		{"TNX 73 GL", ""},
		{"CQ DX", ""},
		{"K1ABC", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.Equal(t, tt.want, Caller(tt.msg))
		})
	}
}

// Colors used by testScheme.
var (
	yellow = color.RGBA{R: 0xff, G: 0xff, A: 0xff}
	gray   = color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
)

// testScheme highlights worked calls, and calls worked on the band and
// mode differently, and leaves new calls alone.
var testScheme = Scheme{
	StatusWorked:         {Background: yellow},
	StatusWorkedBandMode: {Foreground: gray},
}

var (
	wsjtx   = Client{ID: "WSJT-X", Addr: netip.MustParseAddrPort("127.0.0.1:50000")}
	k1abc   = logbook.CallBaseIs{Call: "K1ABC"}
	onBand  = logbook.And{k1abc, logbook.Filter{Bands: []string{"20m"}}, logbook.ModeIs{Mode: "FT8"}}
	anyExpr = gomock.Any()
)

// newTest returns a Highlighter over a mock store, and a clock the test
// moves.
func newTest(t *testing.T) (*Highlighter, *mock.MockStore, *time.Time) {
	t.Helper()
	store := mock.NewMockStore(gomock.NewController(t))
	h := New(store, testScheme, time.Minute)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	return h, store, &now
}

func TestStatusOf(t *testing.T) {
	tests := []struct {
		name         string
		total, onBnd int
		band, mode   string
		want         Status
	}{
		{name: "never worked", total: 0, band: "20m", mode: "FT8", want: StatusNew},
		{name: "worked elsewhere", total: 3, onBnd: 0, band: "20m", mode: "FT8", want: StatusWorked},
		{name: "worked on band and mode", total: 3, onBnd: 1, band: "20m", mode: "FT8", want: StatusWorkedBandMode},
		{name: "band unknown", total: 3, want: StatusWorked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, store, _ := newTest(t)
			store.EXPECT().Count(anyExpr, k1abc).Return(tt.total, nil)
			if tt.total > 0 && tt.band != "" {
				store.EXPECT().Count(anyExpr, onBand).Return(tt.onBnd, nil)
			}
			got, err := h.statusOf(t.Context(), "K1ABC/P", tt.band, tt.mode)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStatusOfCaches(t *testing.T) {
	h, store, now := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(0, nil).Times(2)

	for range 3 {
		got, err := h.statusOf(t.Context(), "K1ABC", "20m", "FT8")
		require.NoError(t, err)
		assert.Equal(t, StatusNew, got)
	}
	*now = now.Add(time.Minute + time.Second)
	_, err := h.statusOf(t.Context(), "K1ABC", "20m", "FT8")
	require.NoError(t, err, "an expired entry is looked up again")
}

func TestStatusOfError(t *testing.T) {
	h, store, _ := newTest(t)
	boom := errors.New("boom")
	store.EXPECT().Count(anyExpr, k1abc).Return(0, boom)
	_, err := h.statusOf(t.Context(), "K1ABC", "20m", "FT8")
	require.ErrorIs(t, err, boom)

	store.EXPECT().Count(anyExpr, k1abc).Return(0, nil)
	_, err = h.statusOf(t.Context(), "K1ABC", "20m", "FT8")
	require.NoError(t, err, "a failed lookup is not cached")
}

func TestDecodeHighlights(t *testing.T) {
	h, store, _ := newTest(t)
	h.Status(wsjtx, 14_074_000, "FT8")
	store.EXPECT().Count(anyExpr, k1abc).Return(2, nil)
	store.EXPECT().Count(anyExpr, onBand).Return(0, nil)

	got, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Highlight{Call: "K1ABC", Colors: Colors{Background: yellow}}, got)

	_, ok, err = h.Decode(t.Context(), wsjtx, "W9XYZ K1ABC -10")
	require.NoError(t, err)
	assert.False(t, ok, "an unchanged status is not sent again")
}

func TestDecodeNewCallWithoutColorsSendsNothing(t *testing.T) {
	h, store, _ := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(0, nil)
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestDecodeWithoutCallSendsNothing(t *testing.T) {
	h, _, _ := newTest(t)
	_, ok, err := h.Decode(t.Context(), wsjtx, "TNX 73 GL")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestDecodeResendsAfterTTL(t *testing.T) {
	h, store, now := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(2, nil).Times(2)
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok)

	*now = now.Add(time.Minute + time.Second)
	got, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok, "a highlight is sent again in case the program restarted")
	assert.Equal(t, Colors{Background: yellow}, got.Colors)
}

func TestLoggedChangesHighlight(t *testing.T) {
	h, store, _ := newTest(t)
	h.Status(wsjtx, 14_074_000, "FT8")
	store.EXPECT().Count(anyExpr, k1abc).Return(0, nil)
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.False(t, ok)

	h.Logged("K1ABC/P")
	store.EXPECT().Count(anyExpr, k1abc).Return(1, nil)
	store.EXPECT().Count(anyExpr, onBand).Return(1, nil)
	got, ok, err := h.Decode(t.Context(), wsjtx, "W9XYZ K1ABC RR73")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Highlight{Call: "K1ABC", Colors: Colors{Foreground: gray}}, got)
}

func TestBandChangeClearsHighlight(t *testing.T) {
	h, store, _ := newTest(t)
	h.scheme = Scheme{StatusWorkedBandMode: {Foreground: gray}}
	h.Status(wsjtx, 14_074_000, "FT8")
	store.EXPECT().Count(anyExpr, k1abc).Return(1, nil).Times(2)
	store.EXPECT().Count(anyExpr, onBand).Return(1, nil)
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok)

	h.Status(wsjtx, 7_074_000, "FT8")
	store.EXPECT().Count(anyExpr, logbook.And{k1abc, logbook.Filter{Bands: []string{"40m"}}, logbook.ModeIs{Mode: "FT8"}}).Return(0, nil)
	got, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok, "a highlight the new status has no colors for is cleared")
	assert.Equal(t, Highlight{Call: "K1ABC"}, got)
}

func TestClientsAreSeparate(t *testing.T) {
	h, store, _ := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(2, nil)
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.True(t, ok)

	jtdx := Client{ID: "JTDX", Addr: netip.MustParseAddrPort("127.0.0.1:50001")}
	_, ok, err = h.Decode(t.Context(), jtdx, "CQ K1ABC FN42")
	require.NoError(t, err)
	assert.True(t, ok, "another program gets its own highlight, from the cached status")
}

func TestDecodeError(t *testing.T) {
	h, store, _ := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(0, errors.New("boom"))
	_, ok, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.Error(t, err)
	assert.False(t, ok)
}

func TestIdleClientsAreDropped(t *testing.T) {
	h, store, now := newTest(t)
	store.EXPECT().Count(anyExpr, k1abc).Return(2, nil).AnyTimes()
	_, _, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	require.Len(t, h.clients, 1)

	// WSJT-X restarts on a new port; the old one goes quiet.
	*now = now.Add(clientIdle*time.Minute + time.Second)
	restarted := Client{ID: "WSJT-X", Addr: netip.MustParseAddrPort("127.0.0.1:50002")}
	_, _, err = h.Decode(t.Context(), restarted, "CQ K1ABC FN42")
	require.NoError(t, err)
	assert.Len(t, h.clients, 1)
	assert.Contains(t, h.clients, restarted)
}

func TestSentIsBounded(t *testing.T) {
	h, store, now := newTest(t)
	store.EXPECT().Count(anyExpr, gomock.Any()).Return(0, nil).AnyTimes()
	cl := h.client(wsjtx)
	for i := range maxCached {
		cl.sent["K"+strconv.Itoa(i)+"AB"] = sent{expires: now.Add(-time.Second)}
	}
	_, _, err := h.Decode(t.Context(), wsjtx, "CQ K1ABC FN42")
	require.NoError(t, err)
	assert.Len(t, cl.sent, 1, "expired entries are dropped once there are too many")
}
