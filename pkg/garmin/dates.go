package garmin

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date is a calendar date (no time, no zone), the unit Garmin uses in most
// URLs and query parameters, formatted as YYYY-MM-DD.
type Date struct {
	t time.Time
}

// NewDate builds a Date from a year, month and day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// DateOf truncates a time.Time to its calendar date (in t's location).
func DateOf(t time.Time) Date {
	return NewDate(t.Year(), t.Month(), t.Day())
}

// Today is the current calendar date in the local timezone.
func Today() Date { return DateOf(time.Now()) }

// ParseDate parses a YYYY-MM-DD string.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("garmin: invalid date %q (want YYYY-MM-DD): %w", s, err)
	}
	return Date{t}, nil
}

func (d Date) String() string       { return d.t.Format(dateLayout) }
func (d Date) IsZero() bool         { return d.t.IsZero() }
func (d Date) Time() time.Time      { return d.t }
func (d Date) AddDays(n int) Date   { return Date{d.t.AddDate(0, 0, n)} }
func (d Date) After(o Date) bool    { return d.t.After(o.t) }
func (d Date) Before(o Date) bool   { return d.t.Before(o.t) }
func (d Date) Equal(o Date) bool    { return d.t.Equal(o.t) }
func (d Date) DaysUntil(o Date) int { return int(o.t.Sub(d.t) / (24 * time.Hour)) }

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		*d = Date{}
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	// Some Garmin payloads carry a full timestamp in date fields.
	if len(s) > len(dateLayout) {
		s = s[:len(dateLayout)]
	}
	p, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}

// localTimestamp formats a time the way Garmin expects timestamps in request
// bodies: local wall-clock time, millisecond precision, no timezone suffix.
func localTimestamp(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000")
}

// gmtTimestamp is localTimestamp of the same instant expressed in UTC.
func gmtTimestamp(t time.Time) string {
	return localTimestamp(t.UTC())
}

// parseGarminTime parses the naive timestamps Garmin returns (e.g.
// "2006-01-02 15:04:05" or "2006-01-02T15:04:05.0"), interpreted as UTC.
func parseGarminTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.0",
		"2006-01-02T15:04:05.0",
		"2006-01-02T15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("garmin: unrecognized timestamp %q", s)
}
