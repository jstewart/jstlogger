package lotw

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// User is one callsign from the LoTW activity feed.
type User struct {
	// Call is the upper-case callsign as listed in the feed.
	Call string
	// LastUpload is the UTC time of the callsign's most recent upload.
	LastUpload time.Time
}

// feedTimeLayout is the layout of a row's date and time fields joined by a
// space.
const feedTimeLayout = "2006-01-02 15:04:05"

// Parse reads an LoTW activity feed from r. It returns the users in the
// order they first appear, keeping the latest upload when a callsign is
// listed more than once, and the number of malformed rows it skipped: rows
// with the wrong number of fields, an empty callsign, or a bad date or time
// (which includes a header row). It returns an error only when r cannot be
// read as CSV.
func Parse(r io.Reader) (users []User, skipped int, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	index := make(map[string]int)
	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return users, skipped, nil
		}
		if err != nil {
			return nil, skipped, fmt.Errorf("lotw: parse: %w", err)
		}
		u, ok := parseRecord(record)
		if !ok {
			skipped++
			continue
		}
		if i, seen := index[u.Call]; seen {
			if u.LastUpload.After(users[i].LastUpload) {
				users[i].LastUpload = u.LastUpload
			}
			continue
		}
		index[u.Call] = len(users)
		users = append(users, u)
	}
}

// parseRecord converts one CALL,YYYY-MM-DD,HH:MM:SS record, reporting false
// if it is malformed.
func parseRecord(record []string) (User, bool) {
	if len(record) != 3 {
		return User{}, false
	}
	call := strings.ToUpper(strings.TrimSpace(record[0]))
	if call == "" {
		return User{}, false
	}
	at, err := time.Parse(feedTimeLayout,
		strings.TrimSpace(record[1])+" "+strings.TrimSpace(record[2]))
	if err != nil {
		return User{}, false
	}
	return User{Call: call, LastUpload: at.UTC()}, true
}
