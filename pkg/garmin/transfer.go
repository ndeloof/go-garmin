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

// UploadService pushes activity files to Garmin Connect.
type UploadService struct{ c *Client }

// DownloadService fetches activity files from Garmin Connect.
type DownloadService struct{ c *Client }

// UploadResult is the detailedImportResult payload of an upload.
type UploadResult struct {
	DetailedImportResult struct {
		UploadID  json.Number `json:"uploadId"`
		FileName  string      `json:"fileName"`
		Successes []struct {
			InternalID json.Number `json:"internalId"`
		} `json:"successes"`
		Failures []struct {
			InternalID json.Number `json:"internalId"`
			Messages   []struct {
				Code    int    `json:"code"`
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"failures"`
	} `json:"detailedImportResult"`
}

var validUploadExts = map[string]bool{".fit": true, ".gpx": true, ".tcx": true}

// Activity uploads an activity file (FIT, GPX or TCX — detected from
// filename). A duplicate returns an error matching ErrDuplicateUpload.
func (s *UploadService) Activity(ctx context.Context, filename string, data io.Reader) (*UploadResult, error) {
	return s.upload(ctx, "/upload-service/upload", filename, data)
}

// Import uploads an activity file flagged as an import (not re-exported to
// connected services like Strava). A duplicate (HTTP 409) returns an error
// matching ErrDuplicateUpload.
func (s *UploadService) Import(ctx context.Context, filename string, data io.Reader) (*UploadResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	return s.upload(ctx, "/upload-service/upload/"+ext, filename, data)
}

func (s *UploadService) upload(ctx context.Context, path, filename string, data io.Reader) (*UploadResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if !validUploadExts[ext] {
		return nil, fmt.Errorf("garmin: unsupported upload format %q (want .fit, .gpx or .tcx)", ext)
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
	var res UploadResult
	h := http.Header{"Accept": {"*/*"}}
	if err := s.c.do(ctx, http.MethodPost, path, nil, mw.FormDataContentType(), buf.Bytes(), h, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ExportFormat selects a download-service export format.
type ExportFormat string

const (
	ExportTCX ExportFormat = "tcx"
	ExportGPX ExportFormat = "gpx"
	ExportKML ExportFormat = "kml"
	ExportCSV ExportFormat = "csv" // splits CSV
)

// OriginalActivity downloads the original uploaded file of an activity, as a
// ZIP archive containing the device file (usually FIT).
func (s *DownloadService) OriginalActivity(ctx context.Context, activityID int64) ([]byte, error) {
	return s.c.download(ctx, fmt.Sprintf("/download-service/files/activity/%d", activityID), nil)
}

// ExportActivity downloads an activity converted to the given format.
func (s *DownloadService) ExportActivity(ctx context.Context, activityID int64, format ExportFormat) ([]byte, error) {
	return s.c.download(ctx, fmt.Sprintf("/download-service/export/%s/activity/%d", format, activityID), nil)
}
