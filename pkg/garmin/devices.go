package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// DevicesService accesses device-service and web-gateway device endpoints.
type DevicesService struct{ c *Client }

// Device is a registered Garmin device (essential fields).
type Device struct {
	DeviceID               int64       `json:"deviceId"`
	UnitID                 int64       `json:"unitId"`
	ProductDisplayName     string      `json:"productDisplayName"`
	Model                  string      `json:"model,omitempty"`
	SerialNumber           string      `json:"serialNumber,omitempty"`
	PartNumber             string      `json:"partNumber,omitempty"`
	SoftwareVersion        json.Number `json:"softwareVersion,omitempty"`
	LastUploadTime         string      `json:"lastUploadTime,omitempty"`
	ImageURL               string      `json:"imageUrl,omitempty"`
	DeviceTypePk           int64       `json:"deviceTypePk,omitempty"`
	PrimaryTrainingCapable bool        `json:"primaryTrainingCapable,omitempty"`
	CorporateDevice        bool        `json:"corporateDevice,omitempty"`
}

// List returns the registered devices.
func (s *DevicesService) List(ctx context.Context) ([]Device, error) {
	var devices []Device
	err := s.c.getJSON(ctx, "/device-service/deviceregistration/devices", nil, &devices)
	return devices, err
}

// Settings returns the settings of one device.
func (s *DevicesService) Settings(ctx context.Context, deviceID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/device-service/deviceservice/device-info/settings/%d", deviceID), nil, &raw)
	return raw, err
}

// LastUsed returns the last-used device payload.
func (s *DevicesService) LastUsed(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/device-service/deviceservice/mylastused", nil, &raw)
	return raw, err
}

// PrimaryTrainingDevice returns the primary-training-device payload.
func (s *DevicesService) PrimaryTrainingDevice(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/web-gateway/device-info/primary-training-device", nil, &raw)
	return raw, err
}

// SolarData returns the solar input of a solar-capable device over
// [start, end].
func (s *DevicesService) SolarData(ctx context.Context, deviceID int64, start, end Date) (json.RawMessage, error) {
	single := strconv.FormatBool(start.Equal(end))
	path := fmt.Sprintf("/web-gateway/solar/%d/%s/%s", deviceID, start, end)
	var res struct {
		DeviceSolarInput json.RawMessage `json:"deviceSolarInput"`
	}
	if err := s.c.getJSON(ctx, path, url.Values{"singleDayView": {single}}, &res); err != nil {
		return nil, err
	}
	return res.DeviceSolarInput, nil
}

// Alarms aggregates the alarms configured on every registered device.
func (s *DevicesService) Alarms(ctx context.Context) ([]json.RawMessage, error) {
	devices, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	var alarms []json.RawMessage
	for _, d := range devices {
		raw, err := s.Settings(ctx, d.DeviceID)
		if err != nil {
			return nil, err
		}
		var settings struct {
			Alarms []json.RawMessage `json:"alarms"`
		}
		if err := json.Unmarshal(raw, &settings); err == nil {
			alarms = append(alarms, settings.Alarms...)
		}
	}
	return alarms, nil
}
