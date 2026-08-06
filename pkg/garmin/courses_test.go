package garmin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCoursesListAndGet(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/course-service/course", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{{"courseId": 1, "courseName": "Locquirec"}})
	})
	mux.HandleFunc("/course-service/course/1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"courseId": 1, "courseName": "Locquirec"})
	})

	list, err := c.Courses.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !strings.Contains(string(list[0]), "Locquirec") {
		t.Fatalf("List = %v", list)
	}
	got, err := c.Courses.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Locquirec") {
		t.Fatalf("Get = %s", got)
	}
}

func TestCoursesDelete(t *testing.T) {
	c, mux := setupTest(t)
	deleted := false
	mux.HandleFunc("/course-service/course/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Courses.Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DELETE never reached the server")
	}
}

// TestImportGPXFillsRequiredFields verifies the parse-then-create flow: the
// importer returns a bare draft and ImportGPX must fill the fields
// createCourse validates (privacy rule, activity type, source, start point).
func TestImportGPXFillsRequiredFields(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/course-service/course/import", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("expected multipart form: %v", err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("missing file part: %v", err)
		}
		defer f.Close()
		writeJSON(w, map[string]any{
			"courseId":   nil,
			"courseName": "gpx track name",
			"geoPoints": []map[string]any{
				{"latitude": 48.7, "longitude": -3.8, "distance": 0.0},
				{"latitude": 48.8, "longitude": -3.7, "distance": 100.0},
			},
		})
	})
	mux.HandleFunc("/course-service/course", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		course := jsonBody(t, r)
		if course["courseName"] != "my name" {
			t.Errorf("courseName = %v, want my name", course["courseName"])
		}
		for k, want := range map[string]float64{"rulePK": 2, "activityTypePk": 6, "sourceTypeId": 3} {
			if got, _ := course[k].(float64); got != want {
				t.Errorf("%s = %v, want %v", k, course[k], want)
			}
		}
		start, _ := course["startPoint"].(map[string]any)
		if start == nil || start["latitude"] != 48.7 {
			t.Errorf("startPoint = %v, want first geo point", course["startPoint"])
		}
		writeJSON(w, map[string]any{"courseId": 99, "courseName": "my name"})
	})

	gpx := `<?xml version="1.0"?><gpx><trk><trkseg><trkpt lat="48.7" lon="-3.8"/></trkseg></trk></gpx>`
	created, err := c.Courses.ImportGPX(context.Background(), "test.gpx", strings.NewReader(gpx), "my name")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		CourseID int64 `json:"courseId"`
	}
	if err := json.Unmarshal(created, &meta); err != nil || meta.CourseID != 99 {
		t.Fatalf("created = %s (err %v), want courseId 99", created, err)
	}
}

func TestImportGPXRejectsNonGPX(t *testing.T) {
	c, _ := setupTest(t)
	if _, err := c.Courses.ImportGPX(context.Background(), "route.fit", strings.NewReader("x"), ""); err == nil {
		t.Fatal("expected an error for non-GPX input")
	}
}

func TestCoursePushToDevice(t *testing.T) {
	c, mux := setupTest(t)
	mux.HandleFunc("/course-service/course/5", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"courseId": 5, "courseName": "GRP"})
	})
	mux.HandleFunc("/device-service/devicemessage/messages", func(w http.ResponseWriter, r *http.Request) {
		var msgs []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&msgs); err != nil || len(msgs) != 1 {
			t.Fatalf("bad body: %v", err)
		}
		m := msgs[0]
		if m["messageType"] != "courses" || m["messageUrl"] != "course-service/course/fit/5" {
			t.Errorf("message = %v", m)
		}
		if m["messageName"] != "GRP" {
			t.Errorf("messageName = %v, want GRP", m["messageName"])
		}
		writeJSON(w, []map[string]any{{"messageId": 1, "messageStatus": "new"}})
	})

	raw, err := c.Courses.PushToDevice(context.Background(), 5, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "new") {
		t.Fatalf("push result = %s", raw)
	}
}
