//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func TestActivitiesListAndDetails(t *testing.T) {
	c := testClient(t)
	ctx := testCtx(t)

	count, err := c.Activities.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	t.Logf("account has %d activities", count)

	acts, err := c.Activities.List(ctx, &garmin.ActivityListOptions{
		ListOptions: garmin.ListOptions{Limit: 5},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if count > 0 && len(acts) == 0 {
		t.Fatal("no activities returned despite non-zero count")
	}
	if len(acts) == 0 {
		t.Skip("account has no activities")
	}

	a := acts[0]
	if a.ActivityID == 0 {
		t.Fatalf("first activity has no id: %+v", a)
	}
	t.Logf("latest: #%d %q type=%s distance=%.0fm", a.ActivityID, a.ActivityName, a.ActivityType.TypeKey, a.Distance)

	got, err := c.Activities.Get(ctx, a.ActivityID)
	if err != nil {
		t.Fatalf("Get(%d): %v", a.ActivityID, err)
	}
	if got.ActivityID != a.ActivityID {
		t.Fatalf("Get returned id %d", got.ActivityID)
	}

	details, err := c.Activities.Details(ctx, a.ActivityID, nil)
	if err != nil {
		t.Fatalf("Details(%d): %v", a.ActivityID, err)
	}
	ts := details.Series(garmin.MetricTimestamp)
	t.Logf("details: %d samples, %d descriptors", len(ts), len(details.MetricDescriptors))
}

func TestActivityDownload(t *testing.T) {
	c := testClient(t)
	ctx := testCtx(t)
	last, err := c.Activities.Last(ctx)
	if errors.Is(err, garmin.ErrNotFound) {
		t.Skip("no activities")
	}
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	gpx, err := c.Download.ExportActivity(ctx, last.ActivityID, garmin.ExportGPX)
	if err != nil {
		t.Fatalf("ExportActivity GPX: %v", err)
	}
	if len(gpx) == 0 {
		t.Fatal("empty GPX export")
	}
	t.Logf("GPX export: %d bytes", len(gpx))
}

func TestActivityRenameCycle(t *testing.T) {
	writeEnabled(t)
	c := testClient(t)
	ctx := testCtx(t)
	last, err := c.Activities.Last(ctx)
	if errors.Is(err, garmin.ErrNotFound) {
		t.Skip("no activities")
	}
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	orig := last.ActivityName
	if err := c.Activities.SetName(ctx, last.ActivityID, orig+" [go-garmin test]"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	// Always restore.
	if err := c.Activities.SetName(ctx, last.ActivityID, orig); err != nil {
		t.Fatalf("restoring name: %v", err)
	}
}
