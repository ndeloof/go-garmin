package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

// CoursesService accesses course-service endpoints (GPS courses/routes that
// can be followed on a device).
type CoursesService struct{ c *Client }

// List returns the account's courses (summary DTOs).
func (s *CoursesService) List(ctx context.Context) ([]json.RawMessage, error) {
	var list []json.RawMessage
	err := s.c.getJSON(ctx, "/course-service/course", nil, &list)
	return list, err
}

// Get returns one course's full payload (metadata + geo points).
func (s *CoursesService) Get(ctx context.Context, courseID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/course-service/course/%d", courseID), nil, &raw)
	return raw, err
}

// Create uploads a course payload (the course-service JSON format, as
// returned by Get) and returns the created course.
func (s *CoursesService) Create(ctx context.Context, course any) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/course-service/course", nil, course, &raw)
	return raw, err
}

// Update replaces a course entirely.
func (s *CoursesService) Update(ctx context.Context, courseID int64, course map[string]any) error {
	course["courseId"] = courseID
	return s.c.Do(ctx, http.MethodPut, fmt.Sprintf("/course-service/course/%d", courseID), nil, course, nil)
}

// Delete removes a course. A freshly imported course answers HTTP 429 ("not
// yet ready to delete or update") while the server is still processing it —
// retry after a few seconds.
func (s *CoursesService) Delete(ctx context.Context, courseID int64) error {
	return s.c.Do(ctx, http.MethodDelete, fmt.Sprintf("/course-service/course/%d", courseID), nil, nil, nil)
}

// ExportGPX downloads a course as a GPX file.
func (s *CoursesService) ExportGPX(ctx context.Context, courseID int64) ([]byte, error) {
	return s.c.download(ctx, fmt.Sprintf("/course-service/course/gpx/%d", courseID), nil)
}

// ParseGPX uploads a GPX file to the course importer and returns the parsed
// course DRAFT (courseId is null): the server extracts track points, distance
// and elevation but does not save anything. Pass the draft to Create — or use
// ImportGPX, which chains both.
func (s *CoursesService) ParseGPX(ctx context.Context, filename string, data io.Reader) (json.RawMessage, error) {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != ".gpx" {
		return nil, fmt.Errorf("garmin: unsupported course import format %q (want .gpx)", ext)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, data); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	h := http.Header{"Accept": {"*/*"}}
	if err := s.c.do(ctx, http.MethodPost, "/course-service/course/import", nil, mw.FormDataContentType(), buf.Bytes(), h, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ImportGPX creates a course from a GPX file (parse then save) and returns
// the created course. courseName overrides the GPX track name when non-empty.
func (s *CoursesService) ImportGPX(ctx context.Context, filename string, data io.Reader, courseName string) (json.RawMessage, error) {
	draft, err := s.ParseGPX(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	var course map[string]any
	if err := json.Unmarshal(draft, &course); err != nil {
		return nil, fmt.Errorf("garmin: parsing course import draft: %w", err)
	}
	if courseName != "" {
		course["courseName"] = courseName
	}
	// The importer returns a bare draft; createCourse validates several
	// fields it leaves null. Mirror what the Connect web app fills in:
	// private course, web-import source, WGS84, trail-running by default,
	// start point = first track point.
	setIfNil := func(key string, v any) {
		if course[key] == nil {
			course[key] = v
		}
	}
	setIfNil("rulePK", 2)         // 1-Public, 2-Private, 4-Group
	setIfNil("activityTypePk", 6) // trail_running
	setIfNil("sourceTypeId", 3)   // import
	setIfNil("coordinateSystem", "WGS84")
	if course["startPoint"] == nil {
		if pts, ok := course["geoPoints"].([]any); ok && len(pts) > 0 {
			course["startPoint"] = pts[0]
		}
	}
	return s.Create(ctx, course)
}

// PushToDevice queues a course for delivery to a device (device message),
// mirroring WorkoutsService.PushToDevice.
func (s *CoursesService) PushToDevice(ctx context.Context, courseID, deviceID int64) (json.RawMessage, error) {
	var courseName string
	if c, err := s.Get(ctx, courseID); err == nil {
		var meta struct {
			CourseName string `json:"courseName"`
		}
		_ = json.Unmarshal(c, &meta)
		courseName = meta.CourseName
	}
	body := []map[string]any{{
		"deviceId":    deviceID,
		"messageUrl":  fmt.Sprintf("course-service/course/fit/%d", courseID),
		"messageType": "courses",
		"groupName":   nil,
		"messageName": courseName,
		"priority":    1,
		"fileType":    "FIT",
		"metaDataId":  courseID,
	}}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/device-service/devicemessage/messages", nil, body, &raw)
	return raw, err
}
