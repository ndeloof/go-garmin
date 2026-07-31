// Package fitenc is a minimal FIT file encoder covering the single use case
// go-garmin needs: pushing a weight / body-composition measurement to the
// Garmin Connect upload service (a port of python-garminconnect's
// FitEncoderWeight). It is not a general-purpose FIT encoder.
package fitenc

import (
	"encoding/binary"
	"math"
	"time"
)

// Weight is one body-composition measurement. WeightKG is required; every
// other field is written only when > 0.
type Weight struct {
	Time              time.Time
	WeightKG          float64
	PercentFat        float64
	PercentHydration  float64
	VisceralFatMass   float64 // kg
	BoneMass          float64 // kg
	MuscleMass        float64 // kg
	BasalMet          float64 // kcal/day
	ActiveMet         float64 // kcal/day
	PhysiqueRating    float64
	MetabolicAge      float64 // years
	VisceralFatRating float64
	BMI               float64
}

// FIT base types.
const (
	baseEnum   = 0x00
	baseUint8  = 0x02
	baseUint16 = 0x84
	baseUint32 = 0x86
	baseU32Z   = 0x8C
)

// fitEpoch is 1989-12-31T00:00:00Z, the origin of FIT timestamps.
var fitEpoch = time.Date(1989, 12, 31, 0, 0, 0, 0, time.UTC)

type field struct {
	num, size, baseType byte
	value               uint32
}

// EncodeWeight builds a complete FIT file (header + records + CRC) carrying
// one weight_scale message.
func EncodeWeight(w Weight) []byte {
	ts := uint32(w.Time.UTC().Sub(fitEpoch) / time.Second)

	var body []byte
	// file_id (global 0): type=9 (weight), manufacturer=1 (garmin),
	// product, serial, time_created.
	body = appendMessage(body, 0, 0, []field{
		{0, 1, baseEnum, 9},
		{1, 2, baseUint16, 1},
		{2, 2, baseUint16, 0},
		{3, 4, baseU32Z, 0},
		{4, 4, baseUint32, ts},
	})
	// file_creator (global 49).
	body = appendMessage(body, 1, 49, []field{
		{0, 2, baseUint16, 100}, // software_version
		{1, 1, baseUint8, 0},    // hardware_version
	})
	// weight_scale (global 30).
	fields := []field{
		{253, 4, baseUint32, ts},                    // timestamp
		{0, 2, baseUint16, scaled(w.WeightKG, 100)}, // weight
	}
	add := func(num byte, size byte, base byte, v float64, scale float64) {
		if v > 0 {
			fields = append(fields, field{num, size, base, scaled(v, scale)})
		}
	}
	add(1, 2, baseUint16, w.PercentFat, 100)
	add(2, 2, baseUint16, w.PercentHydration, 100)
	add(3, 2, baseUint16, w.VisceralFatMass, 100)
	add(4, 2, baseUint16, w.BoneMass, 100)
	add(5, 2, baseUint16, w.MuscleMass, 100)
	add(7, 2, baseUint16, w.BasalMet, 4)
	add(8, 1, baseUint8, w.PhysiqueRating, 1)
	add(9, 2, baseUint16, w.ActiveMet, 4)
	add(10, 1, baseUint8, w.MetabolicAge, 1)
	add(11, 1, baseUint8, w.VisceralFatRating, 1)
	add(13, 2, baseUint16, w.BMI, 10)
	body = appendMessage(body, 2, 30, fields)

	// 14-byte header: size, protocol 2.0, profile version, data size,
	// ".FIT", CRC of the first 12 bytes.
	header := make([]byte, 14)
	header[0] = 14
	header[1] = 0x20
	binary.LittleEndian.PutUint16(header[2:], 2132)
	binary.LittleEndian.PutUint32(header[4:], uint32(len(body)))
	copy(header[8:], ".FIT")
	binary.LittleEndian.PutUint16(header[12:], crc16(header[:12]))

	out := append(header, body...)
	crc := crc16(out)
	return binary.LittleEndian.AppendUint16(out, crc)
}

func scaled(v, scale float64) uint32 {
	return uint32(math.Round(v * scale))
}

// appendMessage writes a definition record then its data record.
func appendMessage(out []byte, localType byte, globalNum uint16, fields []field) []byte {
	// Definition record.
	out = append(out, 0x40|localType, 0, 0) // header, reserved, little-endian
	out = binary.LittleEndian.AppendUint16(out, globalNum)
	out = append(out, byte(len(fields)))
	for _, f := range fields {
		out = append(out, f.num, f.size, f.baseType)
	}
	// Data record.
	out = append(out, localType)
	for _, f := range fields {
		switch f.size {
		case 1:
			out = append(out, byte(f.value))
		case 2:
			out = binary.LittleEndian.AppendUint16(out, uint16(f.value))
		case 4:
			out = binary.LittleEndian.AppendUint32(out, f.value)
		}
	}
	return out
}

var crcTable = [16]uint16{
	0x0000, 0xCC01, 0xD801, 0x1400, 0xF001, 0x3C00, 0x2800, 0xE401,
	0xA001, 0x6C00, 0x7800, 0xB401, 0x5000, 0x9C01, 0x8801, 0x4400,
}

// crc16 is the FIT CRC (a CRC-16/ARC variant computed nibble-wise).
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		tmp := crcTable[crc&0xF]
		crc = (crc >> 4) & 0x0FFF
		crc = crc ^ tmp ^ crcTable[b&0xF]
		tmp = crcTable[crc&0xF]
		crc = (crc >> 4) & 0x0FFF
		crc = crc ^ tmp ^ crcTable[(b>>4)&0xF]
	}
	return crc
}
