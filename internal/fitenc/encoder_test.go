package fitenc

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestEncodeWeightStructure(t *testing.T) {
	fit := EncodeWeight(Weight{
		Time:       time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		WeightKG:   72.5,
		PercentFat: 18.2,
		BMI:        21.3,
	})

	if len(fit) < 16 {
		t.Fatalf("file too short: %d bytes", len(fit))
	}
	if fit[0] != 14 {
		t.Fatalf("header size = %d", fit[0])
	}
	if string(fit[8:12]) != ".FIT" {
		t.Fatalf("magic = %q", fit[8:12])
	}
	dataSize := binary.LittleEndian.Uint32(fit[4:8])
	if int(dataSize) != len(fit)-14-2 {
		t.Fatalf("dataSize %d != body %d", dataSize, len(fit)-16)
	}
	// Header CRC covers the first 12 bytes.
	if got := binary.LittleEndian.Uint16(fit[12:14]); got != crc16(fit[:12]) {
		t.Fatalf("header CRC mismatch")
	}
	// File CRC covers header+body; CRC over the whole file including the
	// trailing CRC must be 0.
	if crc16(fit) != 0 {
		t.Fatal("file CRC mismatch")
	}
}

func TestEncodeWeightOmitsZeroFields(t *testing.T) {
	small := EncodeWeight(Weight{Time: time.Unix(1700000000, 0), WeightKG: 70})
	big := EncodeWeight(Weight{
		Time: time.Unix(1700000000, 0), WeightKG: 70,
		PercentFat: 20, MuscleMass: 30, BMI: 22,
	})
	if len(small) >= len(big) {
		t.Fatalf("zero fields should be omitted: %d >= %d", len(small), len(big))
	}
}
