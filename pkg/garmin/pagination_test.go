package garmin

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestChunkDateRange(t *testing.T) {
	start := NewDate(2026, 1, 1)
	end := NewDate(2026, 3, 1) // 60 days inclusive
	chunks := chunkDateRange(start, end, 28)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks: %v", len(chunks), chunks)
	}
	if chunks[0].start.String() != "2026-01-01" || chunks[0].end.String() != "2026-01-28" {
		t.Fatalf("chunk 0 = %v", chunks[0])
	}
	if chunks[1].start.String() != "2026-01-29" || chunks[1].end.String() != "2026-02-25" {
		t.Fatalf("chunk 1 = %v", chunks[1])
	}
	if chunks[2].start.String() != "2026-02-26" || chunks[2].end.String() != "2026-03-01" {
		t.Fatalf("chunk 2 = %v", chunks[2])
	}
	// Whole range covered exactly once.
	if chunkDateRange(start, start, 28)[0] != (dateRange{start, start}) {
		t.Fatal("single-day range")
	}
	if chunkDateRange(end, start, 28) != nil {
		t.Fatal("inverted range should be empty")
	}
}

func TestDailyStepsSplitsRequests(t *testing.T) {
	c, mux := setupTest(t)
	var ranges []string
	mux.HandleFunc("GET /usersummary-service/stats/steps/daily/", func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.URL.Path)
		fmt.Fprint(w, `[{"calendarDate": "2026-01-01", "totalSteps": 100}]`)
	})
	steps, err := c.Summaries.DailySteps(context.Background(), NewDate(2026, 1, 1), NewDate(2026, 2, 15))
	if err != nil {
		t.Fatalf("DailySteps: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 chunked requests, got %v", ranges)
	}
	if len(steps) != 2 {
		t.Fatalf("concatenated %d entries", len(steps))
	}
}

func TestDateJSONRoundTrip(t *testing.T) {
	d := NewDate(2026, 7, 31)
	b, err := d.MarshalJSON()
	if err != nil || string(b) != `"2026-07-31"` {
		t.Fatalf("marshal: %s %v", b, err)
	}
	var out Date
	if err := out.UnmarshalJSON([]byte(`"2026-07-31"`)); err != nil || !out.Equal(d) {
		t.Fatalf("unmarshal: %v %v", out, err)
	}
	// Timestamp-bearing date fields are truncated.
	if err := out.UnmarshalJSON([]byte(`"2026-07-31T08:00:00.0"`)); err != nil || !out.Equal(d) {
		t.Fatalf("unmarshal long: %v %v", out, err)
	}
	if err := out.UnmarshalJSON([]byte(`null`)); err != nil || !out.IsZero() {
		t.Fatalf("unmarshal null: %v %v", out, err)
	}
}

func TestLocalTimestampFormat(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	ts := time.Date(2026, 7, 31, 10, 30, 0, 0, loc)
	if got := localTimestamp(ts); got != "2026-07-31T10:30:00.000" {
		t.Fatalf("localTimestamp = %q", got)
	}
	if got := gmtTimestamp(ts); got != "2026-07-31T09:30:00.000" {
		t.Fatalf("gmtTimestamp = %q", got)
	}
}
