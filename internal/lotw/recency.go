package lotw

import "time"

// Recency classes how recently a callsign last uploaded to LoTW.
type Recency int

const (
	// Unknown means the callsign is not an LoTW user, or there is no data.
	Unknown Recency = iota
	// Recent means the callsign uploaded within RecentWithin.
	Recent
	// Active means the callsign uploaded within ActiveWithin, but not
	// within RecentWithin.
	Active
	// Inactive means the callsign last uploaded more than ActiveWithin ago.
	Inactive
)

const (
	// RecentWithin is the most time since a callsign's last upload for it
	// to count as Recent.
	RecentWithin = 30 * 24 * time.Hour
	// ActiveWithin is the most time since a callsign's last upload for it
	// to count as Active.
	ActiveWithin = 365 * 24 * time.Hour
)

// RecencyOf classes a last upload at last, as seen at now. A zero last
// means no upload is known and gives Unknown. An upload after now counts as
// Recent.
func RecencyOf(last, now time.Time) Recency {
	if last.IsZero() {
		return Unknown
	}
	switch age := now.Sub(last); {
	case age <= RecentWithin:
		return Recent
	case age <= ActiveWithin:
		return Active
	default:
		return Inactive
	}
}

// DaysAgo returns the number of whole days from last to now, rounded down
// and never negative.
func DaysAgo(last, now time.Time) int {
	return max(0, int(now.Sub(last)/(24*time.Hour)))
}
