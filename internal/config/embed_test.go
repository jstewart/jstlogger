package config

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantDefaultTemplate is the "Default Config File" from issue #2, verbatim.
const wantDefaultTemplate = `# jstlogger configuration
# Run ` + "`jstlogger --print-default-config`" + ` to see every available option.

default_station = "home"

[stations.home]
name       = "Home QTH"
callsign   = ""   # Required, e.g. "N0CALL"
gridsquare = ""   # Required, e.g. "CM87WJ"
`

// wantFullExample is the "Full Configuration Example" from issue #2, verbatim.
const wantFullExample = `default_station = "home"   # Station used when --station is not given

[stations.home]
name        = "Home QTH"
callsign    = "N0CALL"
dxcc        = 291          # ADIF entity code (291 = USA)
cq_zone     = 3
itu_zone    = 6
gridsquare  = "CM87WJ"
state       = "CA"         # Primary subdivision; only for some countries
city        = "Your City"
rig_profile = "ic7300"     # Falls back to rig.profile when unset

[stations.portable]
name        = "Portable"
callsign    = "N0CALL/P"
dxcc        = 291
cq_zone     = 3
itu_zone    = 6
gridsquare  = "CM88"
rig_profile = "portable"
# Default xOTA references for this station.
# Override at runtime with --pota, --sota, --iota, --wwff.
pota_references = ["US-1234", "US-5678"]   # Multiple parks ("two-fer")
sota_reference  = "W6/NC-001"
iota_reference  = "NA-146"
wwff_reference  = "KFF-1234"

[rig]
profile       = "ic7300"   # Used when the station sets no rig_profile
poll_interval = "1s"

[rig.profiles.ic7300]
backend = "rigctld"        # "none" | "rigctld"  (later: "flrig")
host    = "localhost"
port    = 4532

  # Optional: let jstlogger start rigctld itself.
  # Leave this out if you already run rigctld (e.g. for WSJT-X).
  [rig.profiles.ic7300.spawn]
  model  = 3073                       # Hamlib model number; list with ` + "`rigctl -l`" + `
  device = "/dev/cu.SLAB_USBtoUART"   # Linux: /dev/ttyUSB0
  baud   = 19200

[rig.profiles.portable]
backend = "rigctld"
host    = "localhost"
port    = 4533
`

func TestDefaultTemplateMatchesIssue(t *testing.T) {
	assert.Equal(t, wantDefaultTemplate, string(DefaultTemplate))
}

func TestFullExampleMatchesIssue(t *testing.T) {
	assert.Equal(t, wantFullExample, string(FullExample))
}

func TestWriteFullExample(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteFullExample(&buf))
	assert.Equal(t, wantFullExample, buf.String())
}

func TestFullExampleLoads(t *testing.T) {
	c, res := load(t, string(FullExample))

	assert.Empty(t, res.Warnings)
	assert.Equal(t, Config{
		DefaultStation: "home",
		Stations: map[string]Station{
			"home": {
				Name:       "Home QTH",
				Callsign:   "N0CALL",
				DXCC:       291,
				CQZone:     3,
				ITUZone:    6,
				Gridsquare: "CM87WJ",
				State:      "CA",
				City:       "Your City",
				RigProfile: "ic7300",
			},
			"portable": {
				Name:           "Portable",
				Callsign:       "N0CALL/P",
				DXCC:           291,
				CQZone:         3,
				ITUZone:        6,
				Gridsquare:     "CM88",
				RigProfile:     "portable",
				POTAReferences: []string{"US-1234", "US-5678"},
				SOTAReference:  "W6/NC-001",
				IOTAReference:  "NA-146",
				WWFFReference:  "KFF-1234",
			},
		},
		Rig: Rig{
			Profile:      "ic7300",
			PollInterval: time.Second,
			Profiles: map[string]RigProfile{
				"ic7300": {
					Backend: BackendRigctld,
					Host:    "localhost",
					Port:    4532,
					Spawn:   &Spawn{Model: 3073, Device: "/dev/cu.SLAB_USBtoUART", Baud: 19200},
				},
				"portable": {Backend: BackendRigctld, Host: "localhost", Port: 4533},
			},
		},
	}, c)
}
