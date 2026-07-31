package garmin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

func TestListActivitiesBareArray(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("activityType") != "running" {
			t.Errorf("activityType = %q", r.URL.Query().Get("activityType"))
		}
		fmt.Fprint(w, `[{"activityId": 1, "activityName": "Morning Run", "distance": 10000, "activityType": {"typeKey": "running"}}]`)
	})
	acts, err := c.Activities.List(context.Background(), &ActivityListOptions{ActivityType: "running"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(acts) != 1 || acts[0].ActivityName != "Morning Run" || acts[0].ActivityType.TypeKey != "running" {
		t.Fatalf("acts = %+v", acts)
	}
}

func TestListActivitiesWrappedObject(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"activityList": [{"activityId": 2, "activityName": "Ride"}]}`)
	})
	acts, err := c.Activities.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(acts) != 1 || acts[0].ActivityID != 2 {
		t.Fatalf("acts = %+v", acts)
	}
}

func TestAllActivitiesPaginates(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		total := 250
		fmt.Fprint(w, "[")
		for i := start; i < min(start+limit, total); i++ {
			if i > start {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"activityId": %d}`, i)
		}
		fmt.Fprint(w, "]")
	})
	var ids []int64
	for a, err := range c.Activities.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, a.ActivityID)
	}
	if len(ids) != 250 || ids[0] != 0 || ids[249] != 249 {
		t.Fatalf("got %d ids (first=%v last=%v)", len(ids), ids[0], ids[len(ids)-1])
	}
}

func TestAllActivitiesEarlyBreak(t *testing.T) {
	c, mux := setupTest(t)
	var pages int
	mux.HandleFunc("/activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		pages++
		fmt.Fprint(w, `[`)
		for i := 0; i < 100; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"activityId": %d}`, i)
		}
		fmt.Fprint(w, `]`)
	})
	count := 0
	for _, err := range c.Activities.All(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		count++
		if count == 10 {
			break
		}
	}
	if pages != 1 {
		t.Fatalf("early break still fetched %d pages", pages)
	}
}

func TestActivityDetailsSeries(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/activity-service/activity/42/details", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("maxChartSize") != "2000" {
			t.Errorf("maxChartSize = %q", r.URL.Query().Get("maxChartSize"))
		}
		fmt.Fprint(w, `{
			"activityId": 42,
			"metricDescriptors": [
				{"metricsIndex": 0, "key": "directTimestamp"},
				{"metricsIndex": 1, "key": "directHeartRate"}
			],
			"activityDetailMetrics": [
				{"metrics": [1700000000000, 120]},
				{"metrics": [1700000001000, null]},
				{"metrics": [1700000002000, 140]}
			]
		}`)
	})
	d, err := c.Activities.Details(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	hr := d.Series(MetricHeartRate)
	if len(hr) != 3 || *hr[0] != 120 || hr[1] != nil || *hr[2] != 140 {
		t.Fatalf("hr series = %v", hr)
	}
	if d.Series("nonexistent") != nil {
		t.Fatal("unknown metric should yield nil")
	}
}

func TestCountActivities(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/activitylist-service/activities/count", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"totalCount": 1234}`)
	})
	n, err := c.Activities.Count(context.Background())
	if err != nil || n != 1234 {
		t.Fatalf("Count = %d, %v", n, err)
	}
}
