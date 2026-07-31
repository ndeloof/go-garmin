//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

// yesterday gives wellness data a chance to be complete.
func yesterday() garmin.Date { return garmin.Today().AddDays(-1) }

func TestDailySummary(t *testing.T) {
	c := testClient(t)
	ds, err := c.Summaries.Daily(testCtx(t), yesterday())
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if !ds.CalendarDate.Equal(yesterday()) {
		t.Fatalf("calendarDate = %s", ds.CalendarDate)
	}
	if ds.TotalSteps != nil {
		t.Logf("steps yesterday: %d", *ds.TotalSteps)
	}
}

func TestSleep(t *testing.T) {
	c := testClient(t)
	sd, err := c.Wellness.Sleep(testCtx(t), yesterday())
	if err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if sd.DailySleepDTO.SleepTimeSeconds != nil {
		t.Logf("sleep: %ds", *sd.DailySleepDTO.SleepTimeSeconds)
	}
}

func TestHeartRates(t *testing.T) {
	c := testClient(t)
	hr, err := c.Wellness.HeartRates(testCtx(t), yesterday())
	if err != nil {
		t.Fatalf("HeartRates: %v", err)
	}
	t.Logf("heart-rate samples: %d (resting %v)", len(hr.HeartRateValues), ptrOr(hr.RestingHeartRate, -1))
}

func TestBodyBattery(t *testing.T) {
	c := testClient(t)
	raw, err := c.Wellness.BodyBattery(testCtx(t), yesterday(), garmin.Today())
	if err != nil {
		t.Fatalf("BodyBattery: %v", err)
	}
	if len(raw) == 0 {
		t.Error("empty body battery payload")
	}
}

func TestDailySteps(t *testing.T) {
	c := testClient(t)
	// A >28-day range exercises the chunked fetch against the real cap.
	steps, err := c.Summaries.DailySteps(testCtx(t), garmin.Today().AddDays(-40), garmin.Today())
	if err != nil {
		t.Fatalf("DailySteps: %v", err)
	}
	if len(steps) < 30 {
		t.Fatalf("expected ~41 entries, got %d", len(steps))
	}
}

func TestDevices(t *testing.T) {
	c := testClient(t)
	devices, err := c.Devices.List(testCtx(t))
	if err != nil {
		t.Fatalf("Devices.List: %v", err)
	}
	for _, d := range devices {
		t.Logf("device: %s (id %d)", d.ProductDisplayName, d.DeviceID)
	}
}

func TestHydrationCycle(t *testing.T) {
	writeEnabled(t)
	c := testClient(t)
	ctx := testCtx(t)
	if _, err := c.Wellness.AddHydration(ctx, 200, timeNow()); err != nil {
		t.Fatalf("AddHydration: %v", err)
	}
	// Revert.
	if _, err := c.Wellness.AddHydration(ctx, -200, timeNow()); err != nil {
		t.Fatalf("reverting hydration: %v", err)
	}
}

func TestNotFoundMapping(t *testing.T) {
	c := testClient(t)
	_, err := c.Activities.Get(testCtx(t), 1) // ancient id owned by someone else
	if err == nil {
		t.Skip("activity 1 unexpectedly accessible")
	}
	if !errors.Is(err, garmin.ErrNotFound) && !errors.Is(err, garmin.ErrUnauthorized) {
		t.Fatalf("want 404/403 sentinel, got %v", err)
	}
}
