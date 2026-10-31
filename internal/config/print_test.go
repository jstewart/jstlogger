package config

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTOML runs WriteTOML on c and returns the output.
func writeTOML(t *testing.T, c Config) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, WriteTOML(&buf, c))
	return buf.String()
}

func TestWriteTOMLRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		config func(t *testing.T) Config
	}{
		{name: "full example", config: fullConfig},
		{name: "default template", config: func(t *testing.T) Config {
			c, _ := load(t, string(DefaultTemplate))
			return c
		}},
		{name: "logbook path and unit set", config: func(t *testing.T) Config {
			c := fullConfig(t)
			c.Logbook = Logbook{Path: "/data/log.db", DistanceUnit: DistanceUnitMi}
			return c
		}},
		{name: "session effective config", config: func(t *testing.T) Config {
			return resolve(t, Overrides{Station: "portable", Grid: "FN31", POTA: []string{"US-0001"}, NoRig: true}).Config
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.config(t)
			got, res := load(t, writeTOML(t, want))
			assert.Empty(t, res.Warnings)
			assert.Equal(t, want, got)
			assert.Equal(t, want.Validate("") == nil, got.Validate("") == nil)
		})
	}
}

func TestWriteTOMLValidConfigStaysValid(t *testing.T) {
	c := fullConfig(t)
	got, _ := load(t, writeTOML(t, c))
	assert.NoError(t, got.Validate(""))
}

func TestWriteTOMLPollIntervalIsDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: time.Second, want: `poll_interval = '1s'`},
		{d: 500 * time.Millisecond, want: `poll_interval = '500ms'`},
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			c := fullConfig(t)
			c.Rig.PollInterval = tt.d
			assert.Contains(t, writeTOML(t, c), tt.want)
		})
	}
}

func TestWriteTOMLOmitsNilSpawn(t *testing.T) {
	c := fullConfig(t)
	out := writeTOML(t, c)
	assert.Contains(t, out, "[rig.profiles.ic7300.spawn]")
	assert.NotContains(t, out, "[rig.profiles.portable.spawn]")
}

func TestWriteTOMLLogbook(t *testing.T) {
	tests := []struct {
		name    string
		logbook Logbook
		want    []string
		notWant []string
	}{
		{
			name:    "empty path is omitted",
			logbook: Logbook{DistanceUnit: DistanceUnitKm},
			want:    []string{"[logbook]", "distance_unit = 'km'"},
			notWant: []string{"path ="},
		},
		{
			name:    "path is written when set",
			logbook: Logbook{Path: "/data/log.db", DistanceUnit: DistanceUnitMi},
			want:    []string{"[logbook]", "path = '/data/log.db'", "distance_unit = 'mi'"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			c.Logbook = tt.logbook
			out := writeTOML(t, c)
			for _, w := range tt.want {
				assert.Contains(t, out, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, out, w)
			}
		})
	}
}

// failWriter fails every write.
type failWriter struct{}

var errWrite = errors.New("write failed")

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestWriteTOMLWriteError(t *testing.T) {
	err := WriteTOML(failWriter{}, fullConfig(t))
	require.Error(t, err)
	assert.ErrorIs(t, err, errWrite)
}
