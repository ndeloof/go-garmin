package garmin

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestHRVSummaryParsesKnownStatus(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/hrv-service/hrv/2026-09-28", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"hrvSummary": {"status": "UNBALANCED", "weeklyAvg": 42, "lastNightAvg": 21}}`)
	})
	status, err := c.Wellness.HRVSummary(context.Background(), NewDate(2026, time.September, 28))
	if err != nil {
		t.Fatalf("HRVSummary: %v", err)
	}
	if status.Status != HRVStatusUnbalanced {
		t.Errorf("Status = %q, want %q", status.Status, HRVStatusUnbalanced)
	}
	if status.WeeklyAvg == nil || *status.WeeklyAvg != 42 {
		t.Errorf("WeeklyAvg = %v, want 42", status.WeeklyAvg)
	}
	if status.LastNightAvg == nil || *status.LastNightAvg != 21 {
		t.Errorf("LastNightAvg = %v, want 21", status.LastNightAvg)
	}
}

func TestHRVSummaryNoDataReturnsEmptyStatusNotError(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/hrv-service/hrv/2026-09-28", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	status, err := c.Wellness.HRVSummary(context.Background(), NewDate(2026, time.September, 28))
	if err != nil {
		t.Fatalf("HRVSummary: %v", err)
	}
	if status.Status != "" {
		t.Errorf("Status = %q, want empty (no hrvSummary in payload)", status.Status)
	}
}
